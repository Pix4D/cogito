package cogito_test

import "github.com/Pix4D/cogito/cogito"

var (
	baseGithubSource = cogito.Source{
		Owner:       "the-owner",
		Repo:        "the-repo",
		AccessToken: "the-token",
	}
	baseGchatSource = cogito.Source{
		Sinks:        []string{"gchat"},
		GChatWebHook: "https://dummy-webhook",
	}
)
