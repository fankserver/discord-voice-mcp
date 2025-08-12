# Implementation Plan: Fix SSRC Username Mapping

## Quick Fix (Can Deploy Today)

### Step 1: Add SSRC to Transcripts
```go
// internal/session/transcript.go
type Transcript struct {
    Timestamp time.Time
    UserID    string
    Username  string
    SSRC      uint32  // ADD THIS
    Text      string
}
```

### Step 2: Update Buffer to Pass SSRC
```go
// internal/audio/smart_buffer.go
// When creating segments, include SSRC:
segment := &AudioSegment{
    SSRC: b.ssrc,  // Pass SSRC through
    // ... other fields
}
```

### Step 3: Retroactive Username Updates
```go
// internal/bot/bot.go
// In voiceSpeakingUpdate, after mapping SSRC:
if wasUnmapped {
    // Update all existing transcripts with this SSRC
    vb.updateTranscriptsForSSRC(ssrc, nickname)
}

func (vb *VoiceBot) updateTranscriptsForSSRC(ssrc uint32, username string) {
    // Get current session from audio processor
    // Update all transcripts where SSRC matches and name is "Unknown-*"
}
```

## Medium-Term Fix (1-2 Days)

### Dynamic Name Resolution
Instead of storing usernames in buffers, always look them up dynamically:

```go
// internal/audio/async_processor.go
type SmartUserBuffer struct {
    ssrc         uint32
    userResolver UserResolver  // Reference, not copy
}

// When adding transcript:
userID, username, _ := buffer.userResolver.GetUserBySSRC(buffer.ssrc)
sessionManager.AddTranscript(sessionID, userID, username, text)
```

## Long-Term Fix (1 Week)

### Proactive SSRC Discovery
1. When bot joins, enumerate all users in channel
2. Make API calls to get their current voice states
3. Send probe packets to trigger speaking events
4. Implement SSRC pattern detection

## Recommended Approach

**Start with Quick Fix** - It solves the immediate problem:
1. Transcripts will update retroactively when users are identified
2. No major architecture changes needed
3. Can be tested and deployed quickly

**Then implement Medium-Term Fix** for better real-time accuracy:
1. Names will be correct as soon as SSRC is mapped
2. No need for retroactive updates
3. Cleaner architecture

**Consider Long-Term Fix** only if needed:
1. Evaluate if Quick + Medium fixes are sufficient
2. Long-term fix adds complexity
3. May not be worth it if most users get identified quickly

## Test Plan

1. Deploy Quick Fix
2. Join channel with 3+ users already present
3. Have each user speak
4. Verify transcripts update from "Unknown-XXX" to actual names
5. Check new transcripts use correct names immediately

## Files to Modify

Quick Fix requires changes to:
- `internal/session/transcript.go` - Add SSRC field
- `internal/session/manager.go` - Add update method
- `internal/audio/smart_buffer.go` - Pass SSRC in segment
- `internal/audio/async_processor.go` - Pass SSRC to session
- `internal/bot/bot.go` - Call update method after mapping

Total: ~50 lines of code changes