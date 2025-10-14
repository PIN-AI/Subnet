package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	agentsdk "github.com/PIN-AI/subnet-sdk/go"
)

// WeatherRequest represents a weather query request
type WeatherRequest struct {
	City    string `json:"city"`
	Country string `json:"country,omitempty"`
	Days    int    `json:"days,omitempty"`
}

// WeatherResponse represents weather data
type WeatherResponse struct {
	City        string    `json:"city"`
	Country     string    `json:"country"`
	Temperature float64   `json:"temperature"`
	Humidity    int       `json:"humidity"`
	Conditions  string    `json:"conditions"`
	WindSpeed   float64   `json:"wind_speed"`
	Timestamp   time.Time `json:"timestamp"`
	Forecast    []Forecast `json:"forecast,omitempty"`
}

// Forecast represents a daily forecast
type Forecast struct {
	Date        string  `json:"date"`
	TempHigh    float64 `json:"temp_high"`
	TempLow     float64 `json:"temp_low"`
	Conditions  string  `json:"conditions"`
	Rainfall    float64 `json:"rainfall_mm"`
}

// WeatherAgent provides weather data services
type WeatherAgent struct {
	// In production, this would connect to a real weather API
	apiKey string
}

// Execute implements the Handler interface
func (w *WeatherAgent) Execute(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	// Only handle weather tasks
	if task.Type != "weather.query" && task.Type != "weather.forecast" {
		return &agentsdk.Result{
			Success: false,
			Error:   fmt.Sprintf("unsupported task type: %s", task.Type),
		}, nil
	}

	log.Printf("Processing weather task %s", task.ID)

	// Parse request
	var req WeatherRequest
	if err := json.Unmarshal(task.Data, &req); err != nil {
		return nil, fmt.Errorf("invalid request format: %w", err)
	}

	if req.City == "" {
		return nil, fmt.Errorf("city is required")
	}

	// Get weather data (simulated for demo)
	var response interface{}

	switch task.Type {
	case "weather.query":
		response = w.getCurrentWeather(req)
	case "weather.forecast":
		if req.Days <= 0 {
			req.Days = 5
		}
		response = w.getForecast(req, req.Days)
	}

	// Simulate API call delay
	select {
	case <-time.After(500 * time.Millisecond):
		// Processing complete
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Encode response
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to encode response: %w", err)
	}

	log.Printf("Weather data for %s retrieved successfully", req.City)

	return &agentsdk.Result{
		Data:    data,
		Success: true,
		Metadata: map[string]string{
			"source": "simulated-weather-api",
			"city":   req.City,
		},
	}, nil
}

// getCurrentWeather returns current weather (simulated)
func (w *WeatherAgent) getCurrentWeather(req WeatherRequest) *WeatherResponse {
	// In production, this would call a real weather API
	conditions := []string{"Sunny", "Partly Cloudy", "Cloudy", "Rainy", "Stormy"}

	return &WeatherResponse{
		City:        req.City,
		Country:     req.Country,
		Temperature: 20 + rand.Float64()*15, // 20-35°C
		Humidity:    40 + rand.Intn(40),     // 40-80%
		Conditions:  conditions[rand.Intn(len(conditions))],
		WindSpeed:   5 + rand.Float64()*20,  // 5-25 km/h
		Timestamp:   time.Now(),
	}
}

// getForecast returns weather forecast (simulated)
func (w *WeatherAgent) getForecast(req WeatherRequest, days int) *WeatherResponse {
	resp := w.getCurrentWeather(req)

	// Generate forecast
	resp.Forecast = make([]Forecast, days)
	conditions := []string{"Sunny", "Partly Cloudy", "Cloudy", "Rainy", "Stormy"}

	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, i+1)
		baseTemp := 20 + rand.Float64()*10

		resp.Forecast[i] = Forecast{
			Date:       date.Format("2006-01-02"),
			TempHigh:   baseTemp + rand.Float64()*5,
			TempLow:    baseTemp - rand.Float64()*5,
			Conditions: conditions[rand.Intn(len(conditions))],
			Rainfall:   rand.Float64() * 20, // 0-20mm
		}
	}

	return resp
}

// ShouldBid decides whether to bid on an intent
func (w *WeatherAgent) ShouldBid(intent *agentsdk.Intent) bool {
	// Only bid on weather-related intents
	return intent.Type == "weather.query" || intent.Type == "weather.forecast"
}

// CalculateBid calculates the bid price
func (w *WeatherAgent) CalculateBid(intent *agentsdk.Intent) *agentsdk.Bid {
	// Price based on task type
	price := uint64(50) // Base price

	if intent.Type == "weather.forecast" {
		price = 80 // Forecast costs more
	}

	return &agentsdk.Bid{
		Price:    price,
		Currency: "PIN",
	}
}

func main() {
	// Parse flags
	var (
		agentID     = flag.String("id", "weather-agent-001", "Agent ID")
		matcherAddr = flag.String("matcher", "localhost:8090", "Matcher address")
		apiKey      = flag.String("api-key", "", "Weather API key (optional for demo)")
	)
	flag.Parse()

	// Create SDK configuration
	config := &agentsdk.Config{
		AgentID:     *agentID,
		MatcherAddr: *matcherAddr,
		Capabilities: []string{
			"weather.query",
			"weather.forecast",
			"weather.alerts",
		},
		MaxConcurrentTasks: 10,
		LogLevel:           "info",
	}

	// Create SDK instance
	sdk, err := agentsdk.New(config)
	if err != nil {
		log.Fatalf("Failed to create SDK: %v", err)
	}

	// Create and register handler
	agent := &WeatherAgent{
		apiKey: *apiKey,
	}
	sdk.RegisterHandler(agent)

	// Start the agent
	if err := sdk.Start(); err != nil {
		log.Fatalf("Failed to start SDK: %v", err)
	}

	log.Printf("Weather Agent started successfully")
	log.Printf("Agent ID: %s", config.AgentID)
	log.Printf("Capabilities: %v", config.Capabilities)
	log.Printf("Connected to matcher at: %s", *matcherAddr)

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown
	<-sigCh
	log.Println("Shutting down Weather Agent...")

	if err := sdk.Stop(); err != nil {
		log.Printf("Error stopping SDK: %v", err)
	}
}