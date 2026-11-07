package domain

import "testing"

func TestNormalizeFormReadsTextForms(t *testing.T) {
	cases := map[string]struct {
		content  string
		expected []Parameter
	}{
		"encoded query": {
			content:  "title=Hello+world&tags=foo%2Cbar",
			expected: []Parameter{{Name: "title", Value: "Hello world", Enabled: true}, {Name: "tags", Value: "foo,bar", Enabled: true}},
		},
		"bruno lines": {
			content: "  title: test\n  published: false\n  ~draft: yes\n  url: https://example.com/a?b=c\n",
			expected: []Parameter{
				{Name: "title", Value: "test", Enabled: true},
				{Name: "published", Value: "false", Enabled: true},
				{Name: "draft", Value: "yes", Enabled: false},
				{Name: "url", Value: "https://example.com/a?b=c", Enabled: true},
			},
		},
		"one bruno line": {
			content:  "slug: test8",
			expected: []Parameter{{Name: "slug", Value: "test8", Enabled: true}},
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			body := Body{Type: BodyFormURLEncoded, Content: testCase.content}
			body.NormalizeForm()
			if body.Content != "" || len(body.Fields) != len(testCase.expected) {
				t.Fatalf("unexpected body: %+v", body)
			}
			for index, field := range testCase.expected {
				if body.Fields[index] != field {
					t.Errorf("field %d: expected %+v, got %+v", index, field, body.Fields[index])
				}
			}
		})
	}
}

func TestNormalizeFormLeavesOtherBodiesAlone(t *testing.T) {
	body := Body{Type: BodyText, Content: "title: test"}
	body.NormalizeForm()
	if body.Content != "title: test" || body.Fields != nil {
		t.Errorf("a text body must not change, got %+v", body)
	}
}

func TestPathParameterNamesSkipSchemesAndPorts(t *testing.T) {
	names := PathParameterNames("https://api.example.com:8080/articles/:articleId/tags/:tag/:articleId")
	if len(names) != 2 || names[0] != "articleId" || names[1] != "tag" {
		t.Errorf("unexpected names: %v", names)
	}
	if names := PathParameterNames("{{base_url}}api/articles"); len(names) != 0 {
		t.Errorf("expected no path parameter, got %v", names)
	}
}

func TestURLPlaceholdersInPathAndQuery(t *testing.T) {
	placeholders := URLPlaceholders("{{base_url}}/api/articles/:articleId?query=:todoId&time=12:30&partial=a:b&last=:articleId#top")

	type found struct {
		name    string
		inQuery bool
	}
	expected := []found{{"articleId", false}, {"todoId", true}, {"articleId", true}}
	if len(placeholders) != len(expected) {
		t.Fatalf("unexpected placeholders: %+v", placeholders)
	}
	for index, placeholder := range placeholders {
		if placeholder.Name != expected[index].name || placeholder.InQuery != expected[index].inQuery {
			t.Errorf("placeholder %d: expected %+v, got %+v", index, expected[index], placeholder)
		}
	}
	if names := PathParameterNames("/a/:x?b=:y&c=:x"); len(names) != 2 || names[0] != "x" || names[1] != "y" {
		t.Errorf("expected each name once, got %v", names)
	}
}
