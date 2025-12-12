package tui

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

// CodeBlock represents a parsed code block
type CodeBlock struct {
	Language string
	Content  string
	Index    int // Index for copy reference
}

// ParsedContent contains text segments and code blocks
type ParsedContent struct {
	Segments []ContentSegment
}

// ContentSegment represents either text or a code block
type ContentSegment struct {
	IsCode    bool
	Text      string
	CodeBlock *CodeBlock
}

var (
	// Match fenced code blocks: ```lang\ncode\n``` or ```\ncode\n```
	codeBlockRegex = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)\\s*\\n?(.*?)\\n?```")
	// Match inline code: `code`
	inlineCodeRegex = regexp.MustCompile("`([^`]+)`")
)

// ParseContent extracts code blocks and text from content
func ParseContent(content string) *ParsedContent {
	result := &ParsedContent{
		Segments: make([]ContentSegment, 0),
	}

	codeIndex := 0
	lastEnd := 0
	matches := codeBlockRegex.FindAllStringSubmatchIndex(content, -1)

	for _, match := range matches {
		// match[0:2] is full match, match[2:4] is language, match[4:6] is code content
		if match[0] > lastEnd {
			// Add text before this code block
			textSegment := content[lastEnd:match[0]]
			if strings.TrimSpace(textSegment) != "" {
				result.Segments = append(result.Segments, ContentSegment{
					IsCode: false,
					Text:   textSegment,
				})
			}
		}

		// Extract language and code
		lang := ""
		if match[2] >= 0 && match[3] > match[2] {
			lang = content[match[2]:match[3]]
		}
		code := ""
		if match[4] >= 0 && match[5] > match[4] {
			code = content[match[4]:match[5]]
		}

		codeIndex++
		result.Segments = append(result.Segments, ContentSegment{
			IsCode: true,
			CodeBlock: &CodeBlock{
				Language: lang,
				Content:  strings.TrimSpace(code),
				Index:    codeIndex,
			},
		})

		lastEnd = match[1]
	}

	// Add remaining text after last code block
	if lastEnd < len(content) {
		textSegment := content[lastEnd:]
		if strings.TrimSpace(textSegment) != "" {
			result.Segments = append(result.Segments, ContentSegment{
				IsCode: false,
				Text:   textSegment,
			})
		}
	}

	// If no code blocks were found, return the whole content as text
	if len(result.Segments) == 0 {
		result.Segments = append(result.Segments, ContentSegment{
			IsCode: false,
			Text:   content,
		})
	}

	return result
}

// GetCodeBlocks returns all code blocks from parsed content
func (p *ParsedContent) GetCodeBlocks() []*CodeBlock {
	blocks := make([]*CodeBlock, 0)
	for _, seg := range p.Segments {
		if seg.IsCode && seg.CodeBlock != nil {
			blocks = append(blocks, seg.CodeBlock)
		}
	}
	return blocks
}

// RenderCodeBlock renders a code block with styling and syntax highlighting
func RenderCodeBlock(styles Styles, block *CodeBlock, width int) string {
	if width < 20 {
		width = 20
	}

	// Calculate inner width (accounting for border and padding)
	innerWidth := width - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	var sb strings.Builder

	// Header with language and copy hint - compact format
	langDisplay := block.Language
	if langDisplay == "" {
		langDisplay = "code"
	}
	copyHint := fmt.Sprintf("/copy %d", block.Index)

	header := styles.CodeBlockLang.Render(langDisplay) + " " + styles.CodeBlockCopy.Render(copyHint)
	sb.WriteString(header)
	sb.WriteString("\n")

	// Apply syntax highlighting
	highlightedCode := highlightCode(block.Content, block.Language, innerWidth)
	sb.WriteString(highlightedCode)

	// Wrap in code block border (compact style)
	return styles.CodeBlock.Width(width).Render(sb.String())
}

// highlightCode applies syntax highlighting to code
func highlightCode(code, language string, maxWidth int) string {
	// Get lexer for the language
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	// Use a dark terminal style
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}

	// Create terminal true color formatter for best color support
	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		formatter = formatters.Get("terminal256")
	}
	if formatter == nil {
		formatter = formatters.Fallback
	}

	// Tokenize and format
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return renderPlainCode(code, maxWidth)
	}

	var buf bytes.Buffer
	err = formatter.Format(&buf, style, iterator)
	if err != nil {
		return renderPlainCode(code, maxWidth)
	}

	result := buf.String()
	result = strings.TrimSuffix(result, "\n")

	return result
}

// renderPlainCode renders code without syntax highlighting (fallback)
func renderPlainCode(code string, maxWidth int) string {
	var sb strings.Builder
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		// Truncate long lines
		if len(line) > maxWidth {
			line = line[:maxWidth-3] + "..."
		}
		sb.WriteString(line)
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// RenderInlineCode renders inline code with styling
func RenderInlineCode(styles Styles, code string) string {
	return styles.InlineCode.Render(code)
}

// RenderTextWithInlineCode renders text, styling any inline code
func RenderTextWithInlineCode(styles Styles, text string, textStyle lipgloss.Style) string {
	// Find all inline code matches
	matches := inlineCodeRegex.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return textStyle.Render(text)
	}

	var result strings.Builder
	lastEnd := 0

	for _, match := range matches {
		// Add text before inline code
		if match[0] > lastEnd {
			result.WriteString(textStyle.Render(text[lastEnd:match[0]]))
		}

		// Add styled inline code (match[2:4] is the captured group without backticks)
		if match[2] >= 0 && match[3] > match[2] {
			code := text[match[2]:match[3]]
			result.WriteString(RenderInlineCode(styles, code))
		}

		lastEnd = match[1]
	}

	// Add remaining text
	if lastEnd < len(text) {
		result.WriteString(textStyle.Render(text[lastEnd:]))
	}

	return result.String()
}
