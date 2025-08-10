# Voice Activity Detection (VAD) Improvements

## Problem Summary
The Discord voice transcription system was cutting off sentences mid-speech and creating unnatural breaks due to overly aggressive timeout settings.

## Issues Identified
1. **Sentences cut mid-speech**: "Fängst du erst an zu arbeiten, wenn ich aufhöre zu" 
2. **Too short silence timeouts**: 800ms and 1500ms are too short for natural thinking pauses
3. **Forced cutoffs**: 10-second max segments causing interruptions mid-sentence
4. **Poor language mixing**: German/English switches confusing the VAD

## Changes Made

### IntelligentVAD Configuration (`internal/audio/intelligent_vad.go`)
**Before:**
```go
MinSpeechDuration:  500ms    // Good - keep
MaxSilenceInSpeech: 300ms    // Too short for natural pauses
SentenceEndSilence: 800ms    // Too short for thought boundaries  
MaxSegmentDuration: 10s      // Causes mid-sentence cutoffs
TargetDuration:     3s       // Too aggressive
EnergyDropRatio:    0.4      // 40% drop threshold
```

**After:**
```go
MinSpeechDuration:  500ms    // Unchanged - good baseline
MaxSilenceInSpeech: 500ms    // Increased from 300ms - allow mid-sentence pauses
SentenceEndSilence: 2500ms   // Increased from 800ms - detect real thought boundaries
MaxSegmentDuration: 20s      // Increased from 10s - prevent forced cutoffs
TargetDuration:     5s       // Increased from 3s - allow complete thoughts
EnergyDropRatio:    0.3      // Decreased from 0.4 - more sensitive to pauses
```

### Main Processor Configuration (`internal/audio/processor.go`)
**Before:**
```go
defaultBufferDurationSec    = 3     // 3 seconds
defaultSilenceTimeoutMs     = 1500  // 1.5 seconds
defaultContextExpirationSec = 10    // 10 seconds
```

**After:**
```go
defaultBufferDurationSec    = 5     // 5 seconds - more complete thoughts
defaultSilenceTimeoutMs     = 2500  // 2.5 seconds - match VAD sentence detection
defaultContextExpirationSec = 15    // 15 seconds - longer context retention
```

## VAD Decision Hierarchy
The system now uses this priority order for transcription triggers:

1. **Priority 1 (Urgent)**: Maximum segment duration reached (20s) - prevent infinite segments
2. **Priority 2 (High)**: Natural sentence ending detected (2.5s silence) - **primary trigger**
3. **Priority 3 (Normal)**: Target duration reached with pause (5s + 0.5s pause) - balanced approach
4. **Priority 4 (High)**: Extended silence detected (5s silence) - handle very long pauses

## Balanced Settings (Current)

After the initial fix was too aggressive, updated to balanced values:

### IntelligentVAD Configuration (Balanced)
```go
MinSpeechDuration:  500ms    // 0.5s min speech (unchanged)
MaxSilenceInSpeech: 400ms    // 0.4s max mid-sentence pause (balanced)
SentenceEndSilence: 1200ms   // 1.2s silence for sentence end (balanced from 800ms/2500ms)
MaxSegmentDuration: 15s      // 15s max segment (balanced from 10s/20s)
TargetDuration:     4s       // 4s target duration (balanced from 3s/5s)
EnergyDropRatio:    0.35     // 35% energy drop threshold (balanced)
```

### Main Processor Configuration (Balanced)
```go
defaultBufferDurationSec    = 4     // 4 seconds (balanced from 3s/5s)
defaultSilenceTimeoutMs     = 1200  // 1.2 seconds (balanced from 1500ms/2500ms)
defaultContextExpirationSec = 12    // 12 seconds (balanced from 10s/15s)
```

## Environment Variables

You can now tune VAD settings without rebuilding by setting environment variables:

### Voice Activity Detection
```bash
# Speech detection timing (in milliseconds)
VAD_MIN_SPEECH_MS=500                    # Minimum speech duration before considering transcription
VAD_MAX_SILENCE_IN_SPEECH_MS=400         # Maximum silence allowed mid-sentence  
VAD_SENTENCE_END_SILENCE_MS=1200         # Silence duration to detect sentence end

# Segment duration (in seconds)
VAD_MAX_SEGMENT_DURATION_S=15            # Maximum segment duration (forced cutoff)
VAD_TARGET_DURATION_S=4                  # Ideal segment duration

# Energy detection 
VAD_ENERGY_DROP_RATIO=0.35               # Energy drop threshold (0.0-1.0)
VAD_MIN_ENERGY_LEVEL=100.0               # Minimum energy to consider as speech
```

### Example Usage
```bash
# For more responsive sentence detection (shorter pauses)
docker run -e VAD_SENTENCE_END_SILENCE_MS=800 discord-voice-mcp:balanced

# For longer thoughts (less aggressive cutting)
docker run -e VAD_SENTENCE_END_SILENCE_MS=1800 -e VAD_TARGET_DURATION_S=6 discord-voice-mcp:balanced

# For very short, rapid fire responses
docker run -e VAD_SENTENCE_END_SILENCE_MS=600 -e VAD_TARGET_DURATION_S=2 discord-voice-mcp:balanced
```

## Expected Results
- **Balanced sentence detection**: Not too aggressive (like original) or too slow (like first fix)
- **Natural pauses**: 1.2s allows for thinking time without long delays
- **Complete thoughts**: 4s target captures phrases without forced cutoffs
- **Tunable**: Environment variables allow real-time adjustment without rebuilding
- **Better responsiveness**: Shorter than 2.5s timeout but longer than 800ms original

## Debug Logging Added
New logs show both configurations at startup:
- Audio processor configuration (buffer sizes, timeouts)  
- IntelligentVAD configuration (speech detection parameters)
- Environment variable values (when overridden)

This allows runtime verification of settings and easier debugging of speech detection issues.