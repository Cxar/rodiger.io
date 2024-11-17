package docs

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

type Client struct {
	service *docs.Service
}

func NewClient(credPath string) (*Client, error) {
	ctx := context.Background()

	credentials, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("reading credentials: %w", err)
	}

	config, err := google.JWTConfigFromJSON(credentials, docs.DocumentsReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	service, err := docs.NewService(ctx, option.WithHTTPClient(config.Client(ctx)))
	if err != nil {
		return nil, fmt.Errorf("creating docs service: %w", err)
	}

	return &Client{service: service}, nil
}

func (c *Client) GetDocument(docID string) (string, error) {
	doc, err := c.service.Documents.Get(docID).Do()
	if err != nil {
		return "", fmt.Errorf("fetching document: %w", err)
	}

	var content strings.Builder
	inlineObjects := doc.InlineObjects
	if inlineObjects == nil {
		inlineObjects = make(map[string]docs.InlineObject)
	}

	for _, elem := range doc.Body.Content {
		if elem.Paragraph == nil {
			continue
		}
		c.processParagraph(elem.Paragraph, inlineObjects, &content)
	}

	return content.String(), nil
}

func (c *Client) processParagraph(p *docs.Paragraph, inlineObjects map[string]docs.InlineObject, content *strings.Builder) {
	content.WriteString(c.getParagraphPrefix(p))

	for _, pe := range p.Elements {
		if pe.TextRun != nil {
			c.processTextRun(pe.TextRun, content)
			continue
		}
		if pe.InlineObjectElement != nil {
			c.processInlineObject(pe.InlineObjectElement, inlineObjects, content)
		}
	}
	content.WriteString("\n")
}

func (c *Client) getParagraphPrefix(p *docs.Paragraph) string {
	if p.Bullet != nil {
		return "* "
	}
	if p.ParagraphStyle == nil {
		return ""
	}

	switch p.ParagraphStyle.NamedStyleType {
	case "HEADING_1":
		return "# "
	case "HEADING_2":
		return "## "
	case "HEADING_3":
		return "### "
	default:
		return ""
	}
}

func (c *Client) processTextRun(tr *docs.TextRun, content *strings.Builder) {
	text := tr.Content
	if tr.TextStyle != nil && tr.TextStyle.Link != nil {
		text = fmt.Sprintf("[%s](%s)", strings.TrimSpace(text), tr.TextStyle.Link.Url)
	}
	content.WriteString(text)
}

func (c *Client) processInlineObject(ioe *docs.InlineObjectElement, inlineObjects map[string]docs.InlineObject, content *strings.Builder) {
	obj, ok := inlineObjects[ioe.InlineObjectId]
	if !ok {
		return
	}

	props := obj.InlineObjectProperties
	if props == nil || props.EmbeddedObject == nil || props.EmbeddedObject.ImageProperties == nil {
		return
	}

	imageURI := props.EmbeddedObject.ImageProperties.ContentUri
	if imageURI == "" {
		return
	}

	if strings.HasPrefix(imageURI, "data:image/") {
		if imagePath, err := c.saveBase64Image(imageURI); err == nil {
			content.WriteString(fmt.Sprintf("\n![image](%s)\n", imagePath))
		}
		return
	}
	content.WriteString(fmt.Sprintf("\n![image](%s)\n", imageURI))
}

func (c *Client) saveBase64Image(dataURI string) (string, error) {
	parts := strings.Split(dataURI, ",")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid data URI format")
	}

	mimeType := strings.TrimPrefix(strings.Split(parts[0], ";")[0], "data:")
	ext := ".png"
	switch mimeType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/svg+xml":
		ext = ".svg"
	}

	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decoding base64: %w", err)
	}

	hash := sha256.Sum256(data)
	filename := fmt.Sprintf("%x%s", hash[:8], ext)

	if err := os.MkdirAll(filepath.Join("static", "images"), 0755); err != nil {
		return "", fmt.Errorf("creating images directory: %w", err)
	}

	filepath := filepath.Join("static", "images", filename)
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return "", fmt.Errorf("writing image file: %w", err)
	}

	return "/static/images/" + filename, nil
}
