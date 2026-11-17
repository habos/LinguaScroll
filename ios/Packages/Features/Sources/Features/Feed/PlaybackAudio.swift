import AVFoundation

/// Clips play with sound even when the mute switch is on (spec V6 rule 7: audio session `.playback`).
/// Pausing on background is the feed's job (`FeedViewController.didEnterBackground`).
public enum PlaybackAudio {
    public static func configure(_ session: AVAudioSession = .sharedInstance()) {
        try? session.setCategory(.playback, mode: .moviePlayback)
        try? session.setActive(true)
    }
}
