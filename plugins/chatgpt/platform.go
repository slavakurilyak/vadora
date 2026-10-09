package chatgpt

// Capabilities is the intersection of expected support and host advertisement.
// Filesystem and file features are deliberately absent from this plugin.
type Capabilities struct {
	VisualUI        bool
	Entrypoints     bool
	DeepLinks       bool
	Settings        bool
	Mentions        bool
	Forms           bool
	Thumbnails      bool
	NewConversation bool
	ExternalLinks   bool
}

func PlatformSupport(platform string, host Capabilities) Capabilities {
	cap := Capabilities{}
	for _, p := range []string{"desktop", "web", "ios", "android"} {
		if platform == p {
			cap = host
		}
	}
	if platform == "android" {
		cap.DeepLinks = false
	}
	if platform != "desktop" {
		cap.Mentions = false
	}
	if platform == "ios" {
		cap.Forms = false
		cap.Thumbnails = false
		cap.NewConversation = false
	}
	if platform == "android" {
		cap.Forms = false
		cap.NewConversation = false
	}
	return cap
}
