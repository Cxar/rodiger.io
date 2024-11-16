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

	// Get all inline objects first
	inlineObjects := make(map[string]docs.InlineObject)
	if doc.InlineObjects != nil {
		inlineObjects = doc.InlineObjects
	}

	// Process each element
	for _, elem := range doc.Body.Content {
		if elem.Paragraph != nil {
			for _, pe := range elem.Paragraph.Elements {
				if pe.TextRun != nil {
					text := pe.TextRun.Content
					if pe.TextRun.TextStyle != nil && pe.TextRun.TextStyle.Link != nil {
						url := pe.TextRun.TextStyle.Link.Url
						text = fmt.Sprintf("[%s](%s)", strings.TrimSpace(text), url)
					}
					content.WriteString(text)
				}
				if pe.InlineObjectElement != nil {
					objID := pe.InlineObjectElement.InlineObjectId
					if obj, ok := inlineObjects[objID]; ok {
						// Extract embedded object
						if obj.InlineObjectProperties != nil &&
							obj.InlineObjectProperties.EmbeddedObject != nil {

							// Handle image
							if obj.InlineObjectProperties.EmbeddedObject.ImageProperties != nil {
								imageURI := obj.InlineObjectProperties.EmbeddedObject.ImageProperties.ContentUri
								if imageURI != "" {
									// Handle both base64 and URL images
									if strings.HasPrefix(imageURI, "data:image/") {
										if imagePath, err := c.saveBase64Image(imageURI); err == nil {
											content.WriteString(fmt.Sprintf("\n![image](%s)\n", imagePath))
										}
									} else {
										content.WriteString(fmt.Sprintf("\n![image](%s)\n", imageURI))
									}
								}
							}
						}
					}
				}
			}
			// Add newline after each paragraph
			content.WriteString("\n")
		}
	}

	return content.String(), nil
}

func (c *Client) saveBase64Image(dataURI string) (string, error) {
	// Split the header and data
	parts := strings.Split(dataURI, ",")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid data URI format")
	}

	// Get the image type
	mimeType := strings.TrimPrefix(strings.Split(parts[0], ";")[0], "data:")
	ext := ".png" // default extension
	switch mimeType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/svg+xml":
		ext = ".svg"
	}

	// Decode base64 data
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decoding base64: %w", err)
	}

	// Create a unique filename
	hash := sha256.Sum256(data)
	filename := fmt.Sprintf("%x%s", hash[:8], ext)

	// Ensure images directory exists
	if err := os.MkdirAll(filepath.Join("static", "images"), 0755); err != nil {
		return "", fmt.Errorf("creating images directory: %w", err)
	}

	// Save the file
	filepath := filepath.Join("static", "images", filename)
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return "", fmt.Errorf("writing image file: %w", err)
	}

	// Return the URL path
	return "/static/images/" + filename, nil
}
