package matcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"subnet/internal/logging"
	pb "subnet/proto/subnet"
	rootpb "subnet/proto/rootlayer"
)

// SimpleTaskManager simple task manager
// Principle: Keep it simple, avoid over-engineering
type SimpleTaskManager struct {
	logger logging.Logger
	server *Server

	mu              sync.RWMutex
	pendingTasks    map[string]*PendingTask    // taskID -> task
	agentTasks      map[string][]string        // agentID -> taskIDs
}

// PendingTask represents a pending task
type PendingTask struct {
	TaskID      string
	IntentID    string
	AgentID     string
	Intent      *rootpb.Intent
	CreatedAt   time.Time
	Responded   bool      // Has agent responded
	Accepted    bool      // Did agent accept
}

// NewSimpleTaskManager creates a simple task manager
func NewSimpleTaskManager(logger logging.Logger, server *Server) *SimpleTaskManager {
	return &SimpleTaskManager{
		logger:       logger,
		server:       server,
		pendingTasks: make(map[string]*PendingTask),
		agentTasks:   make(map[string][]string),
	}
}

// CreateAndSendTask creates and sends task (maintains original flow)
func (tm *SimpleTaskManager) CreateAndSendTask(intentID string, winningBid *pb.Bid, intent *rootpb.Intent) error {
	// 1. Check if Intent has expired
	if intent.Deadline > 0 && time.Now().Unix() > intent.Deadline {
		return fmt.Errorf("intent %s already expired", intentID)
	}

	taskID := fmt.Sprintf("task-%s-%d", intentID, time.Now().UnixNano())

	// 2. Record pending task
	pending := &PendingTask{
		TaskID:    taskID,
		IntentID:  intentID,
		AgentID:   winningBid.AgentId,
		Intent:    intent,
		CreatedAt: time.Now(),
	}

	tm.mu.Lock()
	tm.pendingTasks[taskID] = pending
	tm.agentTasks[winningBid.AgentId] = append(tm.agentTasks[winningBid.AgentId], taskID)
	tm.mu.Unlock()

	// 3. Create execution task
	executionTask := &pb.ExecutionTask{
		TaskId:     taskID,
		IntentId:   intentID,
		AgentId:    winningBid.AgentId,
		BidId:      winningBid.BidId,
		CreatedAt:  time.Now().Unix(),
		Deadline:   intent.Deadline,
		IntentData: intent.Params.IntentRaw,
		IntentType: intent.IntentType,
	}

	// 4. Send to agent via existing StreamTasks
	if err := tm.sendTaskToAgent(winningBid.AgentId, executionTask); err != nil {
		tm.logger.Errorf("Failed to send task to agent %s: %v", winningBid.AgentId, err)
		// Send failed, try runner-up
		tm.tryRunnerUp(intentID, taskID, winningBid.AgentId)
		return err
	}

	// 5. Set timeout check (Agent must respond within configured timeout)
	responseTimeout := tm.getTaskResponseTimeout()
	go tm.waitForResponse(taskID, responseTimeout)

	return nil
}

// HandleAgentResponse handles agent response
func (tm *SimpleTaskManager) HandleAgentResponse(response *pb.TaskResponse) error {
	tm.mu.Lock()
	pending, exists := tm.pendingTasks[response.TaskId]
	if !exists {
		tm.mu.Unlock()
		return fmt.Errorf("unknown task %s", response.TaskId)
	}

	pending.Responded = true
	pending.Accepted = response.Accepted
	tm.mu.Unlock()

	if response.Accepted {
		// Agent accepted task
		tm.logger.Infof("Agent %s accepted task %s", response.AgentId, response.TaskId)

		// Submit to RootLayer
		tm.submitToRootLayer(pending)

		// Start monitoring execution (optional)
		go tm.monitorExecution(response.TaskId, pending.Intent.Deadline)
	} else {
		// Agent rejected task
		tm.logger.Infof("Agent %s rejected task %s: %s",
			response.AgentId, response.TaskId, response.Reason)

		// Immediately try runner-up
		tm.tryRunnerUp(pending.IntentID, response.TaskId, response.AgentId)
	}

	return nil
}

// waitForResponse waits for agent response
func (tm *SimpleTaskManager) waitForResponse(taskID string, timeout time.Duration) {
	time.Sleep(timeout)

	tm.mu.RLock()
	pending, exists := tm.pendingTasks[taskID]
	tm.mu.RUnlock()

	if !exists {
		return
	}

	if !pending.Responded {
		tm.logger.Warnf("Agent %s did not respond to task %s in time",
			pending.AgentID, taskID)

		// Timeout without response, treat as rejection
		tm.tryRunnerUp(pending.IntentID, taskID, pending.AgentID)
	}
}

// tryRunnerUp tries to assign to runner-up agent
func (tm *SimpleTaskManager) tryRunnerUp(intentID, originalTaskID, excludeAgent string) {
	// 1. Check if Intent is still valid
	tm.mu.RLock()
	pending := tm.pendingTasks[originalTaskID]
	tm.mu.RUnlock()

	if pending != nil && pending.Intent.Deadline > 0 {
		if time.Now().Unix() > pending.Intent.Deadline {
			tm.logger.Warnf("Intent %s expired, cannot reassign", intentID)
			return
		}
	}

	// 2. Find runner-up agent
	runnerUp := tm.findRunnerUp(intentID, excludeAgent)
	if runnerUp == nil {
		tm.logger.Errorf("No runner-up for intent %s", intentID)
		// Report failure to RootLayer
		tm.reportIntentFailure(intentID, "No available agents for task execution")
		return
	}

	// 3. Create new task and send
	tm.logger.Infof("Reassigning intent %s to runner-up agent %s",
		intentID, runnerUp.AgentId)

	tm.CreateAndSendTask(intentID, runnerUp, pending.Intent)
}

// findRunnerUp finds runner-up agent
func (tm *SimpleTaskManager) findRunnerUp(intentID string, excludeAgent string) *pb.Bid {
	tm.server.mu.RLock()
	defer tm.server.mu.RUnlock()

	bids := tm.server.bidsByIntent[intentID]
	if len(bids) == 0 {
		return nil
	}

	var best *pb.Bid
	for _, bid := range bids {
		if bid.AgentId == excludeAgent {
			continue
		}
		if bid.Status != pb.BidStatus_BID_STATUS_SUBMITTED {
			continue
		}
		if best == nil || bid.Price < best.Price {
			best = bid
		}
	}

	return best
}

// sendTaskToAgent sends task to agent (uses existing mechanism)
func (tm *SimpleTaskManager) sendTaskToAgent(agentID string, task *pb.ExecutionTask) error {
	// Call server's existing send mechanism
	// Can be StreamTasks or other implemented method

	// Assume server has taskStreams
	tm.server.mu.RLock()
	ch, exists := tm.server.taskStreams[agentID]
	tm.server.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no stream for agent %s", agentID)
	}

	sendTimeout := tm.getTaskSendTimeout()
	select {
	case ch <- task:
		return nil
	case <-time.After(sendTimeout):
		return fmt.Errorf("timeout sending to agent")
	}
}

// submitToRootLayer submits to RootLayer
func (tm *SimpleTaskManager) submitToRootLayer(pending *PendingTask) {
	assignment := &rootpb.Assignment{
		AssignmentId: pending.TaskID,
		IntentId:     pending.IntentID,
		AgentId:      pending.AgentID,
	}

	// Submit to RootLayer
	tm.logger.Infof("Submitting assignment %s to RootLayer", assignment.AssignmentId)

	if tm.server != nil && tm.server.rootlayerClient != nil && tm.server.rootlayerClient.IsConnected() {
		ctx, cancel := context.WithTimeout(context.Background(), tm.getRootLayerSubmitTimeout())
		defer cancel()

		if err := tm.server.rootlayerClient.SubmitAssignment(ctx, assignment); err != nil {
			tm.logger.Error("Failed to submit assignment to RootLayer",
				"assignment_id", assignment.AssignmentId,
				"error", err)
		}
	}
}

// monitorExecution monitors task execution and handles timeout
func (tm *SimpleTaskManager) monitorExecution(taskID string, deadline int64) {
	// Simple timeout check
	if deadline > 0 {
		timeout := time.Until(time.Unix(deadline, 0))
		if timeout > 0 {
			time.Sleep(timeout)

			tm.mu.RLock()
			pending := tm.pendingTasks[taskID]
			tm.mu.RUnlock()

			if pending != nil && pending.Accepted {
				tm.logger.Warnf("Task %s reached deadline without completion", taskID)

				// Handle timeout - report to RootLayer and try reassignment
				tm.handleTaskTimeout(pending)
			}
		}
	}
}

// handleTaskTimeout handles task execution timeout
func (tm *SimpleTaskManager) handleTaskTimeout(pending *PendingTask) {
	tm.logger.Error("Task execution timeout",
		"task_id", pending.TaskID,
		"agent_id", pending.AgentID,
		"intent_id", pending.IntentID)

	// Report timeout to RootLayer
	if tm.server != nil && tm.server.rootlayerClient != nil && tm.server.rootlayerClient.IsConnected() {
		ctx, cancel := context.WithTimeout(context.Background(), tm.getRootLayerSubmitTimeout())
		defer cancel()

		// Create timeout report (using a map for now since ExecutionTimeout proto doesn't exist yet)
		timeoutReport := map[string]interface{}{
			"assignment_id": pending.TaskID,
			"intent_id":     pending.IntentID,
			"agent_id":      pending.AgentID,
			"timeout_at":    time.Now().Unix(),
			"reason":        "execution_timeout",
		}

		// This would be sent to RootLayer for slashing/penalty
		tm.logger.Info("Reporting execution timeout to RootLayer",
			"assignment_id", pending.TaskID,
			"agent_id", pending.AgentID)

		// In production, we would call:
		// tm.server.rootlayerClient.ReportExecutionTimeout(ctx, timeoutReport)
		// For now, just log it
		tm.logger.Warn("Execution timeout detected",
			"report", timeoutReport)
		_ = ctx
	}

	// Try to reassign to another agent
	// Check if we still have time before Intent deadline
	if pending.Intent != nil && pending.Intent.Deadline > 0 {
		remainingTime := time.Until(time.Unix(pending.Intent.Deadline, 0))
		if remainingTime > tm.getTaskReassignMinTime() {
			// We have enough time to try another agent
			tm.tryRunnerUp(pending.IntentID, pending.TaskID, pending.AgentID)
		} else {
			// Not enough time, report Intent failure
			tm.reportIntentFailure(pending.IntentID,
				fmt.Sprintf("Task timeout with insufficient time for reassignment (agent: %s)", pending.AgentID))
		}
	} else {
		// No deadline, try reassignment anyway
		tm.tryRunnerUp(pending.IntentID, pending.TaskID, pending.AgentID)
	}

	// Clean up the timed-out task
	tm.mu.Lock()
	delete(tm.pendingTasks, pending.TaskID)

	// Remove from agent's task list
	if tasks, exists := tm.agentTasks[pending.AgentID]; exists {
		newTasks := []string{}
		for _, id := range tasks {
			if id != pending.TaskID {
				newTasks = append(newTasks, id)
			}
		}
		tm.agentTasks[pending.AgentID] = newTasks
	}
	tm.mu.Unlock()
}

// CleanupOldTasks cleans up old tasks (run periodically)
func (tm *SimpleTaskManager) CleanupOldTasks() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	for taskID, pending := range tm.pendingTasks {
		// Clean up tasks older than configured interval
		if now.Sub(pending.CreatedAt) > tm.getTaskCleanupInterval() {
			delete(tm.pendingTasks, taskID)

			// Remove from agentTasks
			if tasks, exists := tm.agentTasks[pending.AgentID]; exists {
				newTasks := []string{}
				for _, id := range tasks {
					if id != taskID {
						newTasks = append(newTasks, id)
					}
				}
				tm.agentTasks[pending.AgentID] = newTasks
			}
		}
	}
}

// Helper methods to get configuration values
func (tm *SimpleTaskManager) getTaskResponseTimeout() time.Duration {
	if tm.server != nil && tm.server.cfg != nil && tm.server.cfg.Timeouts != nil {
		return tm.server.cfg.Timeouts.TaskResponseTimeout
	}
	return 30 * time.Second
}

func (tm *SimpleTaskManager) getTaskSendTimeout() time.Duration {
	if tm.server != nil && tm.server.cfg != nil && tm.server.cfg.Timeouts != nil {
		return tm.server.cfg.Timeouts.TaskSendTimeout
	}
	return 5 * time.Second
}

func (tm *SimpleTaskManager) getRootLayerSubmitTimeout() time.Duration {
	if tm.server != nil && tm.server.cfg != nil && tm.server.cfg.Timeouts != nil {
		return tm.server.cfg.Timeouts.RootLayerSubmitTimeout
	}
	return 5 * time.Second
}

func (tm *SimpleTaskManager) getTaskCleanupInterval() time.Duration {
	if tm.server != nil && tm.server.cfg != nil && tm.server.cfg.Timeouts != nil {
		return tm.server.cfg.Timeouts.TaskCleanupInterval
	}
	return time.Hour
}

func (tm *SimpleTaskManager) getTaskReassignMinTime() time.Duration {
	if tm.server != nil && tm.server.cfg != nil && tm.server.cfg.Timeouts != nil {
		return tm.server.cfg.Timeouts.TaskReassignMinTime
	}
	return 30 * time.Second
}

// reportIntentFailure reports intent execution failure to RootLayer
func (tm *SimpleTaskManager) reportIntentFailure(intentID string, reason string) {
	tm.logger.Error("Reporting intent failure to RootLayer",
		"intent_id", intentID,
		"reason", reason)

	if tm.server != nil && tm.server.rootlayerClient != nil && tm.server.rootlayerClient.IsConnected() {
		// Note: Intent status updates are handled by RootLayer based on validation bundles
		// UpdateIntentStatus method is not available in the gRPC service
		tm.logger.Warn("Intent execution failed",
			"intent_id", intentID,
			"reason", reason)
	}

	// Clean up local state
	tm.mu.Lock()
	// Remove all pending tasks for this intent
	for taskID, pending := range tm.pendingTasks {
		if pending.IntentID == intentID {
			delete(tm.pendingTasks, taskID)
			// Also remove from agent's task list
			if tasks, exists := tm.agentTasks[pending.AgentID]; exists {
				newTasks := []string{}
				for _, id := range tasks {
					if id != taskID {
						newTasks = append(newTasks, id)
					}
				}
				tm.agentTasks[pending.AgentID] = newTasks
			}
		}
	}
	tm.mu.Unlock()
}
