package matcher

import (
	"fmt"
	"sync"
	"time"

	rootpb "rootlayer/proto"
	"subnet/internal/logging"
	pb "subnet/proto/subnet"
)

// TaskDistributor interface for task distribution (reserved for future extension)
type TaskDistributor interface {
	// SendTask sends task to agent
	SendTask(agentID string, task *pb.ExecutionTask) error

	// HandleResponse handles agent response
	HandleResponse(response *pb.TaskResponse) error

	// GetStats returns distribution statistics
	GetStats() *DistributorStats
}

// DistributorStats contains distribution statistics
type DistributorStats struct {
	ActiveTasks    int
	PendingTasks   int
	CompletedTasks int
	FailedTasks    int
	AgentCount     int
}

// SimpleTaskDistributor simple task distributor (current implementation)
type SimpleTaskDistributor struct {
	logger logging.Logger
	server *Server
	cfg    *Config

	mu           sync.RWMutex
	activeTasks  map[string]*ActiveTask            // taskID -> task
	agentStreams map[string]chan *pb.ExecutionTask // agentID -> stream

	// Configuration
	timeoutConfig   *TimeoutConfig
	responseTimeout time.Duration // Default fallback
	maxRetries      int           // Default fallback
}

// ActiveTask represents an active task
type ActiveTask struct {
	Task            *pb.ExecutionTask
	Intent          *rootpb.Intent
	AgentID         string
	Status          string // "pending", "accepted", "rejected", "completed"
	CreatedAt       time.Time
	RespondedAt     *time.Time
	Retries         int
	ResponseTimeout time.Duration // Calculated based on intent
}

// NewSimpleTaskDistributor creates a simple task distributor
func NewSimpleTaskDistributor(logger logging.Logger, server *Server) *SimpleTaskDistributor {
	// Get timeout from server config
	responseTimeout := 30 * time.Second // Default
	if server.cfg != nil && server.cfg.Timeouts != nil {
		responseTimeout = server.cfg.Timeouts.TaskResponseTimeout
	}

	return &SimpleTaskDistributor{
		logger:          logger,
		server:          server,
		cfg:             server.cfg,
		activeTasks:     make(map[string]*ActiveTask),
		agentStreams:    make(map[string]chan *pb.ExecutionTask),
		timeoutConfig:   DefaultTimeoutConfig(),
		responseTimeout: responseTimeout,
		maxRetries:      2, // Default fallback
	}
}

// SendTask sends task using existing streaming mechanism
func (d *SimpleTaskDistributor) SendTask(agentID string, task *pb.ExecutionTask) error {
	return d.SendTaskWithPriority(agentID, task, "normal")
}

// SendTaskWithPriority sends task with specific priority
func (d *SimpleTaskDistributor) SendTaskWithPriority(agentID string, task *pb.ExecutionTask, priority string) error {
	// Calculate timeouts based on task properties
	timeoutSettings := d.timeoutConfig.CalculateTimeouts(
		task.IntentType,
		task.Deadline,
		priority,
	)

	// Record active task
	d.mu.Lock()
	d.activeTasks[task.TaskId] = &ActiveTask{
		Task:            task,
		AgentID:         agentID,
		Status:          "pending",
		CreatedAt:       time.Now(),
		Retries:         0,
		ResponseTimeout: timeoutSettings.ResponseTimeout,
	}

	// Get or create agent stream
	stream, exists := d.agentStreams[agentID]
	if !exists {
		stream = make(chan *pb.ExecutionTask, 10)
		d.agentStreams[agentID] = stream
	}
	d.mu.Unlock()

	// Send task
	select {
	case stream <- task:
		d.logger.Debugf("Sent task %s to agent %s (response timeout: %v, max retries: %d)",
			task.TaskId, agentID, timeoutSettings.ResponseTimeout, timeoutSettings.MaxRetries)

		// Start timeout check with calculated timeout
		go d.checkTimeout(task.TaskId, timeoutSettings.ResponseTimeout)

		// Store max retries for this task
		if timeoutSettings.MaxRetries < d.maxRetries {
			d.activeTasks[task.TaskId].Retries = d.maxRetries - timeoutSettings.MaxRetries
		}

		return nil

	case <-time.After(d.getSendTimeout()):
		return fmt.Errorf("timeout sending task to agent %s", agentID)
	}
}

// HandleResponse handles agent response
func (d *SimpleTaskDistributor) HandleResponse(response *pb.TaskResponse) error {
	d.mu.Lock()
	active, exists := d.activeTasks[response.TaskId]
	if !exists {
		d.mu.Unlock()
		return fmt.Errorf("unknown task %s", response.TaskId)
	}

	now := time.Now()
	active.RespondedAt = &now

	if response.Accepted {
		active.Status = "accepted"
		d.mu.Unlock()

		d.logger.Debugf("Task %s accepted by agent %s", response.TaskId, response.AgentId)

		// Submit to RootLayer
		d.submitToRootLayer(active)

	} else {
		active.Status = "rejected"
		intentID := active.Task.IntentId
		d.mu.Unlock()

		d.logger.Debugf("Task %s rejected by agent %s: %s",
			response.TaskId, response.AgentId, response.Reason)

		// Try to reassign
		d.reassignTask(response.TaskId, intentID, response.AgentId)
	}

	return nil
}

// checkTimeout checks if task has timed out
func (d *SimpleTaskDistributor) checkTimeout(taskID string, timeout time.Duration) {
	time.Sleep(timeout)

	d.mu.Lock()
	active, exists := d.activeTasks[taskID]
	if !exists {
		d.mu.Unlock()
		return
	}

	// If no response yet
	if active.RespondedAt == nil {
		intentID := active.Task.IntentId
		agentID := active.AgentID
		d.mu.Unlock()

		d.logger.Warnf("Task %s timeout for agent %s", taskID, agentID)

		// Treat as rejection and try to reassign
		d.reassignTask(taskID, intentID, agentID)
	} else {
		d.mu.Unlock()
	}
}

// reassignTask reassigns task to another agent
func (d *SimpleTaskDistributor) reassignTask(taskID, intentID, excludeAgent string) {
	d.mu.Lock()
	active, exists := d.activeTasks[taskID]
	if !exists {
		d.mu.Unlock()
		return
	}

	// Check retry count
	if active.Retries >= d.maxRetries {
		d.mu.Unlock()
		d.logger.Errorf("Task %s failed after %d retries", taskID, active.Retries)
		// Report failure to RootLayer
		d.reportFailureToRootLayer(active.Intent, taskID, fmt.Sprintf("Task failed after %d retries", active.Retries))
		return
	}

	active.Retries++
	d.mu.Unlock()

	// Find runner-up agent
	runnerUp := d.findRunnerUp(intentID, excludeAgent)
	if runnerUp == nil {
		d.logger.Errorf("No runner-up agent for task %s", taskID)
		// Report failure to RootLayer
		d.mu.Lock()
		if active, ok := d.activeTasks[taskID]; ok {
			d.reportFailureToRootLayer(active.Intent, taskID, "No available agents for retry")
		}
		d.mu.Unlock()
		return
	}

	// Create new task
	newTask := &pb.ExecutionTask{
		TaskId:     fmt.Sprintf("%s-retry-%d", taskID, active.Retries),
		IntentId:   intentID,
		AgentId:    runnerUp.AgentId,
		BidId:      runnerUp.BidId,
		CreatedAt:  time.Now().Unix(),
		Deadline:   active.Task.Deadline,
		IntentData: active.Task.IntentData,
		IntentType: active.Task.IntentType,
	}

	d.logger.Infof("Reassigning task %s to agent %s (retry %d)",
		taskID, runnerUp.AgentId, active.Retries)

	// Recursive send with same priority
	d.SendTaskWithPriority(runnerUp.AgentId, newTask, "normal")
}

// findRunnerUp finds the runner-up agent
func (d *SimpleTaskDistributor) findRunnerUp(intentID string, excludeAgent string) *pb.Bid {
	d.server.mu.RLock()
	defer d.server.mu.RUnlock()

	bids := d.server.bidsByIntent[intentID]
	if len(bids) == 0 {
		return nil
	}

	var best *pb.Bid
	for _, bid := range bids {
		// Skip already tried agent
		if bid.AgentId == excludeAgent {
			continue
		}

		// Find the best one
		if best == nil || bid.Price < best.Price {
			best = bid
		}
	}

	return best
}

// submitToRootLayer submits to RootLayer
func (d *SimpleTaskDistributor) submitToRootLayer(active *ActiveTask) {
	// Submit successful completion to RootLayer
	d.logger.Infof("Submitting task %s to RootLayer", active.Task.TaskId)

	if d.server.rootlayerClient != nil && d.server.rootlayerClient.IsConnected() {
		// Note: Intent status updates are handled by RootLayer based on validation bundles
		// UpdateIntentStatus method is not available in the gRPC service
		d.logger.Info("Intent execution completed",
			"intent_id", active.Intent.IntentId)
	}
}

// reportFailureToRootLayer reports task failure to RootLayer
func (d *SimpleTaskDistributor) reportFailureToRootLayer(intent *rootpb.Intent, taskID string, reason string) {
	if intent == nil {
		return
	}

	d.logger.Error("Reporting task failure to RootLayer",
		"task_id", taskID,
		"intent_id", intent.IntentId,
		"reason", reason)

	if d.server.rootlayerClient != nil && d.server.rootlayerClient.IsConnected() {
		// Note: Intent status updates are handled by RootLayer based on validation bundles
		// UpdateIntentStatus method is not available in the gRPC service
		d.logger.Warn("Intent execution failed",
			"intent_id", intent.IntentId,
			"reason", reason)

		// In production, we would also submit failure evidence/reason
		// For example:
		// d.server.rootlayerClient.SubmitFailureReport(&FailureReport{
		//     IntentID: intent.IntentId,
		//     TaskID: taskID,
		//     Reason: reason,
		//     Timestamp: time.Now().Unix(),
		// })
	}
}

// GetStats returns statistics
func (d *SimpleTaskDistributor) GetStats() *DistributorStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := &DistributorStats{
		ActiveTasks:    0,
		PendingTasks:   0,
		CompletedTasks: 0,
		FailedTasks:    0,
		AgentCount:     len(d.agentStreams),
	}

	for _, task := range d.activeTasks {
		switch task.Status {
		case "pending":
			stats.PendingTasks++
		case "accepted":
			stats.ActiveTasks++
		case "completed":
			stats.CompletedTasks++
		case "rejected":
			stats.FailedTasks++
		}
	}

	return stats
}

// StreamToAgent handles task stream for agent connection
func (d *SimpleTaskDistributor) StreamToAgent(agentID string, stream pb.MatcherService_StreamTasksServer) error {
	d.mu.Lock()
	ch, exists := d.agentStreams[agentID]
	if !exists {
		ch = make(chan *pb.ExecutionTask, 10)
		d.agentStreams[agentID] = ch
	}
	d.mu.Unlock()

	// Cleanup function
	defer func() {
		d.mu.Lock()
		delete(d.agentStreams, agentID)
		d.mu.Unlock()
	}()

	d.logger.Infof("Started task stream for agent %s", agentID)

	// Stream tasks
	for {
		select {
		case task := <-ch:
			if err := stream.Send(task); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// SetTimeoutConfig updates timeout configuration
func (d *SimpleTaskDistributor) SetTimeoutConfig(config *TimeoutConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.timeoutConfig = config
}

// Cleanup removes expired tasks
// Helper methods for getting timeouts from config

func (d *SimpleTaskDistributor) getSendTimeout() time.Duration {
	if d.cfg != nil && d.cfg.Timeouts != nil {
		return d.cfg.Timeouts.TaskSendTimeout
	}
	return 5 * time.Second // Default fallback
}

func (d *SimpleTaskDistributor) getRootLayerTimeout() time.Duration {
	if d.cfg != nil && d.cfg.Timeouts != nil {
		return d.cfg.Timeouts.RootLayerSubmitTimeout
	}
	return 5 * time.Second // Default fallback
}

func (d *SimpleTaskDistributor) getCleanupInterval() time.Duration {
	if d.cfg != nil && d.cfg.Timeouts != nil {
		return d.cfg.Timeouts.TaskCleanupInterval
	}
	return time.Hour // Default fallback
}

func (d *SimpleTaskDistributor) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for taskID, task := range d.activeTasks {
		// Remove tasks older than cleanup interval
		cleanupInterval := d.getCleanupInterval()
		if now.Sub(task.CreatedAt) > cleanupInterval {
			delete(d.activeTasks, taskID)
		}
	}
}
