package platform

import "testing"

func TestMCPCommandQuotesPathsWhenNeeded(t *testing.T) {
	plain := mcpCommand("/Applications/Awake.app/Contents/MacOS/awake", "/Users/me/api", false)
	if plain != "claude mcp add awake -- /Applications/Awake.app/Contents/MacOS/awake mcp --workspace /Users/me/api" {
		t.Errorf("unexpected command: %s", plain)
	}

	quoted := mcpCommand("/opt/Awake/awake", "/Users/me/My API's", true)
	if quoted != `claude mcp add awake -- /opt/Awake/awake mcp --workspace '/Users/me/My API'\''s' --read-only` {
		t.Errorf("unexpected command: %s", quoted)
	}
}
