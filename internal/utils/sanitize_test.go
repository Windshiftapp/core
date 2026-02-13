package utils

import "testing"

func TestSanitizeMarkdownURLs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "JavascriptLink",
			input: "[Click me](javascript:alert(1))",
			want:  "[Click me](#unsafe-link-removed)",
		},
		{
			name:  "JavascriptLinkCaseInsensitive",
			input: "[Click me](JaVaScRiPt:alert(1))",
			want:  "[Click me](#unsafe-link-removed)",
		},
		{
			name:  "VbscriptLink",
			input: "[Click me](vbscript:MsgBox('xss'))",
			want:  "[Click me](#unsafe-link-removed)",
		},
		{
			name:  "DataURILink",
			input: "[Click me](data:text/html,<script>alert(1)</script>)",
			want:  "[Click me](#unsafe-link-removed)",
		},
		{
			name:  "JavascriptImage",
			input: "![img](javascript:alert(1))",
			want:  "![img](#unsafe-link-removed)",
		},
		{
			name:  "DataURIImage",
			input: "![img](data:image/svg+xml,<svg onload=alert(1)>)",
			want:  "![img](#unsafe-link-removed)",
		},
		{
			name:  "JavascriptWithSpaces",
			input: "[x]( javascript:alert(1))",
			want:  "[x](#unsafe-link-removed)",
		},
		{
			name:  "SafeHTTPLink",
			input: "[Click me](https://example.com)",
			want:  "[Click me](https://example.com)",
		},
		{
			name:  "SafeHTTPImage",
			input: "![alt](https://example.com/img.png)",
			want:  "![alt](https://example.com/img.png)",
		},
		{
			name:  "SafeMailtoLink",
			input: "[email](mailto:test@example.com)",
			want:  "[email](mailto:test@example.com)",
		},
		{
			name:  "SafeRelativeLink",
			input: "[page](/about)",
			want:  "[page](/about)",
		},
		{
			name:  "PlainTextWithJavascript",
			input: "javascript:alert(1) is dangerous",
			want:  "javascript:alert(1) is dangerous",
		},
		{
			name:  "MultipleDangerousLinks",
			input: "[a](javascript:x) and [b](data:text/html,y)",
			want:  "[a](#unsafe-link-removed) and [b](#unsafe-link-removed)",
		},
		{
			name:  "EmptyInput",
			input: "",
			want:  "",
		},
		{
			name:  "NoLinks",
			input: "Just a normal comment with no links",
			want:  "Just a normal comment with no links",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeMarkdownURLs(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeMarkdownURLs(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeDescription(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "JavascriptLinkWithBrTag",
			input: "Line 1<br />Line 2\n[Click me](javascript:alert(1))",
			want:  "Line 1<br />Line 2\n[Click me](#unsafe-link-removed)",
		},
		{
			name:  "HTMLStrippedAndMarkdownURLSanitized",
			input: "<b>Bold</b> text with [x](javascript:alert(1))",
			want:  "Bold text with [x](#unsafe-link-removed)",
		},
		{
			name:  "SafeContentUnchanged",
			input: "Normal **markdown** with [safe link](https://example.com)<br />new line",
			want:  "Normal **markdown** with [safe link](https://example.com)<br />new line",
		},
		{
			name:  "EmptyInput",
			input: "",
			want:  "",
		},
		{
			name:  "NullInput",
			input: "null",
			want:  "",
		},
		{
			name:  "DataURISanitized",
			input: "![img](data:text/html,<script>alert(1)</script>)",
			want:  "![img](#unsafe-link-removed)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeDescription(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeDescription(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeCommentContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "HTMLAndMarkdownXSS",
			input: "<script>alert('xss')</script>[Click me](javascript:alert(1))",
			want:  "[Click me](#unsafe-link-removed)",
		},
		{
			name:  "HTMLOnly",
			input: "<b>Bold</b> text",
			want:  "Bold text",
		},
		{
			name:  "MarkdownXSSOnly",
			input: "[x](javascript:alert(document.cookie))",
			want:  "[x](#unsafe-link-removed)",
		},
		{
			name:  "SafeContent",
			input: "Normal **markdown** with [safe link](https://example.com)",
			want:  "Normal **markdown** with [safe link](https://example.com)",
		},
		{
			name:  "EmptyInput",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeCommentContent(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeCommentContent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "EmptyInput",
			input: "",
			want:  "",
		},
		{
			name:  "NoTags",
			input: "plain text",
			want:  "plain text",
		},
		{
			name:  "SimpleTags",
			input: "<b>Bold</b> and <i>italic</i>",
			want:  "Bold and italic",
		},
		{
			name:  "ScriptTagWithContent",
			input: "<script>alert('xss')</script>safe text",
			want:  "safe text",
		},
		{
			name:  "UnclosedImgTag",
			input: "<img src=x onerror=alert(1)",
			want:  "",
		},
		{
			name:  "NestedMalformedScript",
			input: "<scr<script>ipt>alert(1)</script>",
			want:  "ipt&gt;alert(1)",
		},
		{
			name:  "NullByteInjection",
			input: "<scr\x00ipt>alert(1)</script>",
			want:  "alert(1)",
		},
		{
			name:  "ImgWithOnerror",
			input: "<img src=x onerror=alert(1)>",
			want:  "",
		},
		{
			name:  "IframeTag",
			input: `<iframe src="javascript:alert(1)"></iframe>`,
			want:  "",
		},
		{
			name:  "SVGOnload",
			input: `<svg onload=alert(1)>`,
			want:  "",
		},
		{
			name:  "MalformedHTMLMixedWithText",
			input: "Hello <img src=x onerror=alert(1)> world",
			want:  "Hello  world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTMLTags(tt.input)
			if got != tt.want {
				t.Errorf("StripHTMLTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
