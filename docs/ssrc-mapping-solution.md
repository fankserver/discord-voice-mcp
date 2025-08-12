# Discord Voice Bot SSRC-to-Username Mapping Solution

## Problem Summary

The Discord voice bot faces a critical issue where users appear as "Unknown-XXXXX" instead of their proper Discord usernames/nicknames. This occurs due to:

1. **Discord API Limitation**: VoiceSpeakingUpdate events only fire when users change their speaking state (start/stop talking)
2. **Timing Issue**: Audio buffers are created before SSRC mappings are available
3. **No Retroactive Updates**: Once a buffer is created with "Unknown-XXXXX", it doesn't update even after the SSRC is mapped

## Current State Analysis

### What Works
- VoiceSpeakingUpdate handler correctly registered on VoiceConnection
- SSRC mappings work for users who join AFTER the bot
- CUDA acceleration functioning properly for transcription

### What Doesn't Work
- Users already in channel when bot joins remain unknown
- Toggling mute/unmute does NOT trigger events for existing users  
- Even when SSRC gets mapped, in-progress transcripts keep old "Unknown" names

## Solution Architecture

### 1. Dynamic Name Resolution in Audio Buffers

Instead of storing static usernames in buffers, implement dynamic resolution:

```go
// internal/audio/smart_buffer.go modifications
type SmartUserBuffer struct {
    ssrc         uint32
    userResolver UserResolver  // Dynamic resolver instead of static names
    // Remove static userID, username fields
}

func (b *SmartUserBuffer) getCurrentUsername() string {
    if b.userResolver != nil {
        _, _, nickname := b.userResolver.GetUserBySSRC(b.ssrc)
        return nickname
    }
    return fmt.Sprintf("Unknown-%d", b.ssrc)
}
```

### 2. Update AsyncProcessor Buffer Creation

```go
// internal/audio/async_processor.go modifications
func (p *AsyncProcessor) getOrCreateBuffer(...) *SmartUserBuffer {
    // Create buffer with resolver reference, not static names
    buffer = NewSmartUserBufferWithResolver(
        ssrc, 
        userResolver,  // Pass resolver for dynamic lookups
        p.segmentChan,
        p.config.BufferConfig,
        onTranscriptionComplete,
    )
}
```

### 3. Proactive User Discovery

Implement multiple strategies to discover users:

```go
// internal/bot/bot.go additions
func (vb *VoiceBot) proactiveUserDiscovery(guildID, channelID string) {
    // Strategy 1: Request voice states from Discord API
    guild, _ := vb.discord.Guild(guildID)
    for _, vs := range guild.VoiceStates {
        if vs.ChannelID == channelID {
            vb.requestUserSpeakingState(vs.UserID)
        }
    }
    
    // Strategy 2: Send audio probe to trigger responses
    vb.sendSilentAudioProbe()
    
    // Strategy 3: Monitor RTP SSRC patterns
    go vb.monitorSSRCActivity()
}
```

### 4. SSRC Pattern Recognition

Implement heuristics to detect user patterns:

```go
// internal/bot/ssrc_detector.go (new file)
type SSRCDetector struct {
    patterns map[uint32]*AudioPattern
}

func (d *SSRCDetector) analyzePacketTiming(ssrc uint32, packet *discordgo.Packet) {
    // Detect speech patterns unique to users
    // Use timing, packet size, and frequency analysis
}
```

### 5. Retroactive Name Updates

Update names in session history when mappings occur:

```go
// internal/session/manager.go modifications
func (m *Manager) UpdateTranscriptUsernames(sessionID string, ssrc uint32, username string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if session, exists := m.sessions[sessionID]; exists {
        for i, transcript := range session.Transcripts {
            if transcript.SSRC == ssrc && strings.HasPrefix(transcript.Username, "Unknown-") {
                session.Transcripts[i].Username = username
                session.Transcripts[i].UpdatedAt = time.Now()
            }
        }
    }
}
```

## Implementation Priority

### Phase 1: Dynamic Resolution (Immediate)
1. Modify SmartUserBuffer to use dynamic resolver
2. Update AsyncProcessor to pass resolver reference
3. Test with existing SSRC mappings

### Phase 2: Retroactive Updates (Short-term)
1. Add SSRC field to transcript records
2. Implement UpdateTranscriptUsernames in session manager
3. Call update when new SSRC mappings occur

### Phase 3: Proactive Discovery (Medium-term)
1. Implement voice state API calls
2. Add audio probe mechanism
3. Deploy pattern recognition

## Testing Strategy

1. **Unit Tests**: Test dynamic resolution with mock resolvers
2. **Integration Tests**: Verify retroactive updates work correctly
3. **Live Testing**: Test with real Discord servers with multiple users

## Metrics for Success

- Zero "Unknown-XXXXX" usernames after 5 seconds in channel
- 100% user identification for active speakers
- No performance degradation from dynamic lookups

## Alternative Approaches Considered

1. **Database Caching**: Store SSRC mappings persistently
   - Rejected: SSRCs change between sessions
   
2. **Manual Mapping UI**: Let users manually map SSRCs
   - Rejected: Poor user experience
   
3. **Request All Users to Speak**: Prompt via bot message
   - Rejected: Disruptive to conversation flow

## Discord API Workarounds

Since Discord doesn't provide SSRC mappings upfront, we must:
1. Accept initial "Unknown" labels as temporary
2. Update retroactively when possible
3. Use heuristics for faster discovery
4. Cache mappings within sessions

## Next Steps

1. Implement Phase 1 (dynamic resolution) immediately
2. Deploy and monitor for improvements
3. Iterate based on real-world performance
4. Consider filing Discord API feature request for SSRC endpoint