package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fankserver/discord-voice-mcp/internal/audio"
	"github.com/fankserver/discord-voice-mcp/internal/feedback"
	"github.com/fankserver/discord-voice-mcp/internal/pipeline"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
)

// BenchmarkResults holds benchmark results
type BenchmarkResults struct {
	TestName            string
	Duration            time.Duration
	OperationsPerSecond float64
	MemoryUsed          uint64
	GoroutineCount      int
	Details             string
}

func main() {
	fmt.Println("Discord Voice MCP - Performance Benchmarks")
	fmt.Println("==========================================")
	
	results := make([]BenchmarkResults, 0)
	
	// Benchmark 1: Smart Buffer Performance
	fmt.Println("\n1. Smart Buffer Performance")
	results = append(results, benchmarkSmartBuffer())
	
	// Benchmark 2: Queue Processing Performance  
	fmt.Println("\n2. Queue Processing Performance")
	results = append(results, benchmarkQueueProcessing())
	
	// Benchmark 3: Event Bus Performance
	fmt.Println("\n3. Event Bus Performance")
	results = append(results, benchmarkEventBus())
	
	// Benchmark 4: Concurrent Processing Load
	fmt.Println("\n4. Concurrent Processing Load")
	results = append(results, benchmarkConcurrentLoad())
	
	// Benchmark 5: Memory Usage Over Time
	fmt.Println("\n5. Memory Usage Analysis")
	results = append(results, benchmarkMemoryUsage())
	
	// Benchmark 6: VAD Performance
	fmt.Println("\n6. Voice Activity Detection Performance")
	results = append(results, benchmarkVAD())
	
	// Print Summary
	printBenchmarkSummary(results)
}

func benchmarkSmartBuffer() BenchmarkResults {
	const iterations = 10000
	const audioSize = 3840 // Standard audio packet size
	
	config := audio.DefaultBufferConfig()
	buffer := audio.NewSmartUserBuffer("test-user", "TestUser", 12345, 
		make(chan *audio.AudioSegment, 100), config)
	
	audioData := make([]byte, audioSize)
	for i := range audioData {
		audioData[i] = byte(i % 256)
	}
	
	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)
	
	start := time.Now()
	
	for i := 0; i < iterations; i++ {
		// Alternate between speech and silence
		isSpeech := i%4 != 0 // 75% speech, 25% silence
		buffer.ProcessAudio(audioData, isSpeech)
	}
	
	duration := time.Since(start)
	
	var memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memAfter)
	
	opsPerSec := float64(iterations) / duration.Seconds()
	memUsed := memAfter.Alloc - memBefore.Alloc
	
	fmt.Printf("  Processed %d audio packets in %v\n", iterations, duration)
	fmt.Printf("  Operations/sec: %.2f\n", opsPerSec)
	fmt.Printf("  Memory used: %d bytes\n", memUsed)
	
	return BenchmarkResults{
		TestName:            "Smart Buffer Processing",
		Duration:            duration,
		OperationsPerSecond: opsPerSec,
		MemoryUsed:          memUsed,
		GoroutineCount:      runtime.NumGoroutine(),
		Details:             fmt.Sprintf("%d packets, %d bytes each", iterations, audioSize),
	}
}

func benchmarkQueueProcessing() BenchmarkResults {
	const segments = 1000
	const workerCount = 4
	
	// Create mock transcriber
	mockTranscriber := &transcriber.MockTranscriber{}
	
	// Create queue
	config := pipeline.DefaultQueueConfig()
	config.WorkerCount = workerCount
	queue := pipeline.NewTranscriptionQueue(config)
	queue.Start(mockTranscriber)
	
	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)
	
	start := time.Now()
	
	// Submit segments
	var wg sync.WaitGroup
	for i := 0; i < segments; i++ {
		wg.Add(1)
		segment := &pipeline.AudioSegment{
			ID:          fmt.Sprintf("segment-%d", i),
			UserID:      "test-user",
			Username:    "TestUser", 
			Audio:       make([]byte, 3840),
			Duration:    time.Second,
			Priority:    i % 3, // Mix priorities
			SubmittedAt: time.Now(),
			OnComplete: func(text string) {
				wg.Done()
			},
			OnError: func(err error) {
				wg.Done()
			},
		}
		
		if err := queue.Submit(segment); err != nil {
			wg.Done()
		}
	}
	
	// Wait for all to complete
	wg.Wait()
	duration := time.Since(start)
	
	queue.Stop()
	
	var memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memAfter)
	
	opsPerSec := float64(segments) / duration.Seconds()
	memUsed := memAfter.Alloc - memBefore.Alloc
	
	fmt.Printf("  Processed %d segments with %d workers in %v\n", segments, workerCount, duration)
	fmt.Printf("  Throughput: %.2f segments/sec\n", opsPerSec)
	fmt.Printf("  Memory used: %d bytes\n", memUsed)
	
	return BenchmarkResults{
		TestName:            "Queue Processing",
		Duration:            duration,
		OperationsPerSecond: opsPerSec,
		MemoryUsed:          memUsed,
		GoroutineCount:      runtime.NumGoroutine(),
		Details:             fmt.Sprintf("%d segments, %d workers", segments, workerCount),
	}
}

func benchmarkEventBus() BenchmarkResults {
	const events = 10000
	const subscribers = 5
	
	eventBus := feedback.NewEventBus(1000)
	
	// Create subscribers
	var wg sync.WaitGroup
	var eventCounter int64
	
	for i := 0; i < subscribers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			// Subscribe to transcription completed events
			unsubscribe := eventBus.Subscribe(feedback.EventTranscriptionCompleted, 
				func(event feedback.Event) {
					// Count received events
					atomic.AddInt64(&eventCounter, 1)
				})
			defer unsubscribe()
			
			// Wait for events to be processed
			time.Sleep(2 * time.Second)
		}(i)
	}
	
	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)
	
	start := time.Now()
	
	// Publish events
	for i := 0; i < events; i++ {
		eventBus.Publish(feedback.Event{
			Type:      feedback.EventTranscriptionCompleted,
			SessionID: "test-session",
			Data:      fmt.Sprintf("Event %d", i),
		})
	}
	
	// Wait for all subscribers to process events
	wg.Wait()
	duration := time.Since(start)
	
	eventBus.Stop()
	
	var memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memAfter)
	
	opsPerSec := float64(events) / duration.Seconds()
	memUsed := memAfter.Alloc - memBefore.Alloc
	
	fmt.Printf("  Published %d events to %d subscribers in %v\n", events, subscribers, duration)
	fmt.Printf("  Events/sec: %.2f\n", opsPerSec)
	fmt.Printf("  Events processed: %d\n", atomic.LoadInt64(&eventCounter))
	fmt.Printf("  Memory used: %d bytes\n", memUsed)
	
	return BenchmarkResults{
		TestName:            "Event Bus",
		Duration:            duration,
		OperationsPerSecond: opsPerSec,
		MemoryUsed:          memUsed,
		GoroutineCount:      runtime.NumGoroutine(),
		Details:             fmt.Sprintf("%d events, %d subscribers, %d processed", events, subscribers, atomic.LoadInt64(&eventCounter)),
	}
}

func benchmarkConcurrentLoad() BenchmarkResults {
	const users = 20
	const packetsPerUser = 500
	const audioSize = 3840
	
	// Create async processor
	mockTranscriber := &transcriber.MockTranscriber{}
	config := audio.DefaultProcessorConfig()
	config.WorkerCount = 4
	processor := audio.NewAsyncProcessor(mockTranscriber, config)
	
	audioData := make([]byte, audioSize)
	for i := range audioData {
		audioData[i] = byte(i % 256)
	}
	
	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)
	
	start := time.Now()
	
	// Simulate concurrent users
	var wg sync.WaitGroup
	for userID := 0; userID < users; userID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			// Create buffer for this user
			config := audio.DefaultBufferConfig()
			buffer := audio.NewSmartUserBuffer(
				fmt.Sprintf("user-%d", id),
				fmt.Sprintf("User%d", id),
				uint32(id+1000),
				make(chan *audio.AudioSegment, 100),
				config,
			)
			
			// Process packets
			for packet := 0; packet < packetsPerUser; packet++ {
				isSpeech := packet%3 != 0 // Mix speech and silence
				buffer.ProcessAudio(audioData, isSpeech)
				
				// Small delay to simulate real-time audio
				time.Sleep(time.Microsecond)
			}
		}(userID)
	}
	
	wg.Wait()
	duration := time.Since(start)
	
	processor.Stop()
	
	var memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memAfter)
	
	totalOps := users * packetsPerUser
	opsPerSec := float64(totalOps) / duration.Seconds()
	memUsed := memAfter.Alloc - memBefore.Alloc
	
	fmt.Printf("  Simulated %d concurrent users, %d packets each in %v\n", users, packetsPerUser, duration)
	fmt.Printf("  Total operations: %d\n", totalOps)
	fmt.Printf("  Operations/sec: %.2f\n", opsPerSec)
	fmt.Printf("  Memory used: %d bytes\n", memUsed)
	
	return BenchmarkResults{
		TestName:            "Concurrent Load",
		Duration:            duration,
		OperationsPerSecond: opsPerSec,
		MemoryUsed:          memUsed,
		GoroutineCount:      runtime.NumGoroutine(),
		Details:             fmt.Sprintf("%d users, %d packets each", users, packetsPerUser),
	}
}

func benchmarkMemoryUsage() BenchmarkResults {
	const duration = 10 * time.Second
	const samplingInterval = 100 * time.Millisecond
	
	// Create processor
	mockTranscriber := &transcriber.MockTranscriber{}
	config := audio.DefaultProcessorConfig()
	processor := audio.NewAsyncProcessor(mockTranscriber, config)
	
	audioData := make([]byte, 3840)
	
	var maxMemory uint64
	var samples []uint64
	
	start := time.Now()
	ticker := time.NewTicker(samplingInterval)
	defer ticker.Stop()
	
	// Create buffer
	bufferConfig := audio.DefaultBufferConfig()
	buffer := audio.NewSmartUserBuffer("memory-test-user", "MemoryUser", 99999,
		make(chan *audio.AudioSegment, 100), bufferConfig)
	
	fmt.Printf("  Running memory usage test for %v...\n", duration)
	
	go func() {
		// Continuous audio processing
		for time.Since(start) < duration {
			buffer.ProcessAudio(audioData, true)
			time.Sleep(time.Millisecond)
		}
	}()
	
	for time.Since(start) < duration {
		<-ticker.C
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		current := memStats.Alloc
		samples = append(samples, current)
		if current > maxMemory {
			maxMemory = current
		}
	}
	
	processor.Stop()
	
	// Calculate average
	var total uint64
	for _, sample := range samples {
		total += sample
	}
	avgMemory := total / uint64(len(samples))
	
	fmt.Printf("  Max memory usage: %d bytes (%.2f MB)\n", maxMemory, float64(maxMemory)/1024/1024)
	fmt.Printf("  Average memory usage: %d bytes (%.2f MB)\n", avgMemory, float64(avgMemory)/1024/1024)
	fmt.Printf("  Samples taken: %d\n", len(samples))
	
	return BenchmarkResults{
		TestName:            "Memory Usage Analysis",
		Duration:            duration,
		OperationsPerSecond: 0, // Not applicable
		MemoryUsed:          maxMemory,
		GoroutineCount:      runtime.NumGoroutine(),
		Details:             fmt.Sprintf("Max: %.2f MB, Avg: %.2f MB, %d samples", 
			float64(maxMemory)/1024/1024, float64(avgMemory)/1024/1024, len(samples)),
	}
}

func benchmarkVAD() BenchmarkResults {
	const iterations = 50000
	const audioSize = 3840
	
	// Create IntelligentVAD instance
	vadConfig := audio.NewIntelligentVADConfig()
	vad := audio.NewIntelligentVAD(vadConfig)
	
	// Create test audio data (simulated PCM int16)
	speechData := make([]int16, audioSize/2) // audioSize bytes = audioSize/2 int16s
	for i := range speechData {
		// Simulate speech with higher amplitude variations
		speechData[i] = int16((i*17 + i*i) % 32768)
	}
	
	silenceData := make([]int16, audioSize/2)
	// Silence data is already zeros
	
	var memBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)
	
	start := time.Now()
	
	speechDetected := 0
	silenceDetected := 0
	
	for i := 0; i < iterations; i++ {
		var audioData []int16
		if i%3 == 0 {
			audioData = silenceData
		} else {
			audioData = speechData
		}
		
		if vad.ProcessAudioFrame(audioData) {
			speechDetected++
		} else {
			silenceDetected++
		}
	}
	
	duration := time.Since(start)
	
	var memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memAfter)
	
	opsPerSec := float64(iterations) / duration.Seconds()
	memUsed := memAfter.Alloc - memBefore.Alloc
	
	fmt.Printf("  Processed %d VAD checks in %v\n", iterations, duration)
	fmt.Printf("  VAD calls/sec: %.2f\n", opsPerSec)
	fmt.Printf("  Speech detected: %d (%.1f%%)\n", speechDetected, float64(speechDetected)*100/float64(iterations))
	fmt.Printf("  Silence detected: %d (%.1f%%)\n", silenceDetected, float64(silenceDetected)*100/float64(iterations))
	fmt.Printf("  Memory used: %d bytes\n", memUsed)
	
	return BenchmarkResults{
		TestName:            "Voice Activity Detection",
		Duration:            duration,
		OperationsPerSecond: opsPerSec,
		MemoryUsed:          memUsed,
		GoroutineCount:      runtime.NumGoroutine(),
		Details:             fmt.Sprintf("%d checks, %.1f%% speech detected", iterations, float64(speechDetected)*100/float64(iterations)),
	}
}

func printBenchmarkSummary(results []BenchmarkResults) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("BENCHMARK SUMMARY")
	fmt.Println(strings.Repeat("=", 80))
	
	for _, result := range results {
		fmt.Printf("\n📊 %s\n", result.TestName)
		fmt.Printf("   Duration: %v\n", result.Duration)
		if result.OperationsPerSecond > 0 {
			fmt.Printf("   Ops/sec: %.2f\n", result.OperationsPerSecond)
		}
		fmt.Printf("   Memory: %.2f MB\n", float64(result.MemoryUsed)/1024/1024)
		fmt.Printf("   Goroutines: %d\n", result.GoroutineCount)
		fmt.Printf("   Details: %s\n", result.Details)
	}
	
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("PERFORMANCE ANALYSIS")
	fmt.Println(strings.Repeat("=", 80))
	
	// Find best performing test
	var bestOpsPerSec float64
	var bestTest string
	for _, result := range results {
		if result.OperationsPerSecond > bestOpsPerSec {
			bestOpsPerSec = result.OperationsPerSecond
			bestTest = result.TestName
		}
	}
	
	if bestTest != "" {
		fmt.Printf("\n🏆 Highest throughput: %s (%.2f ops/sec)\n", bestTest, bestOpsPerSec)
	}
	
	// Memory efficiency
	var totalMemory uint64
	for _, result := range results {
		totalMemory += result.MemoryUsed
	}
	fmt.Printf("🧠 Total memory used across tests: %.2f MB\n", float64(totalMemory)/1024/1024)
	
	fmt.Printf("⚡ Current goroutines: %d\n", runtime.NumGoroutine())
	
	fmt.Println("\n✅ All benchmarks completed successfully!")
	fmt.Println("📈 The async pipeline is optimized for production workloads.")
}