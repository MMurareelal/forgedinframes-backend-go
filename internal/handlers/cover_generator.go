package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"forgedinframes-backend-go/config"
	fb "forgedinframes-backend-go/internal/firebase"
)

var (
	htmlTagRegex       = regexp.MustCompile(`<[^>]*>`)
	svgBlockRegex      = regexp.MustCompile(`(?is)<svg\b[^>]*>.*?</svg>`)
	scriptTagRegex     = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	foreignObjRegex    = regexp.MustCompile(`(?is)<foreignObject\b[^>]*>.*?</foreignObject>`)
	eventAttrRegex     = regexp.MustCompile(`(?i)\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	jsProtocolRegex    = regexp.MustCompile(`(?i)javascript:`)
	slugCleanRegex     = regexp.MustCompile(`[^a-z0-9]+`)
)

// SanitizeSVG validates and strips unsafe scripts/attributes from generated SVG markup.
func SanitizeSVG(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	// Strip markdown fences if present
	trimmed = strings.TrimPrefix(trimmed, "```svg")
	trimmed = strings.TrimPrefix(trimmed, "```xml")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	match := svgBlockRegex.FindString(trimmed)
	if match == "" {
		return "", fmt.Errorf("no valid <svg> element found")
	}

	cleaned := scriptTagRegex.ReplaceAllString(match, "")
	cleaned = foreignObjRegex.ReplaceAllString(cleaned, "")
	cleaned = eventAttrRegex.ReplaceAllString(cleaned, "")
	cleaned = jsProtocolRegex.ReplaceAllString(cleaned, "")
	return cleaned, nil
}

// BuildFallbackCoverSVG produces a clean deterministic unDraw-palette SVG when Gemini is unavailable.
func BuildFallbackCoverSVG(title string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(title))))
	hash := h.Sum32()

	accents := []string{"#00BFA6", "#FF6584", "#009688"}
	primary := accents[int(hash)%len(accents)]
	secondary := accents[int(hash>>4)%len(accents)]
	if secondary == primary {
		secondary = "#FF6584"
		if primary == "#FF6584" {
			secondary = "#00BFA6"
		}
	}

	cx1 := 240 + int(hash%160)
	cx2 := 520 + int((hash>>8)%150)
	cy1 := 220 + int((hash>>12)%120)

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="95 85 698 507" width="698" height="507" fill="none">
  <rect x="115" y="105" width="658" height="467" rx="28" fill="#F2F2F2"/>
  <circle cx="%d" cy="%d" r="96" fill="%s" opacity="0.22"/>
  <circle cx="%d" cy="240" r="64" fill="%s" opacity="0.88"/>
  <path d="M190 485 Q340 365 470 445 T720 405" stroke="#2F2E41" stroke-width="5" stroke-linecap="round"/>
  <rect x="240" y="295" width="210" height="140" rx="16" fill="#FFFFFF" stroke="#E6E6E6" stroke-width="3"/>
  <rect x="268" y="330" width="120" height="12" rx="6" fill="#2F2E41"/>
  <rect x="268" y="356" width="154" height="10" rx="5" fill="%s"/>
  <rect x="268" y="380" width="98" height="10" rx="5" fill="#CCCCCC"/>
  <path d="M555 485 C555 420 605 385 645 385 C685 385 715 420 715 485 Z" fill="#3F3D56"/>
  <circle cx="635" cy="345" r="28" fill="#FFB8B8"/>
  <path d="M607 342 C607 315 628 302 650 308 C664 312 668 330 660 342 Z" fill="#2F2E41"/>
</svg>`, cx1, cy1, primary, cx2, secondary, primary)
}

// GenerateStoryCoverSVG generates a story-specific unDraw-style SVG via Gemini, falling back to a deterministic SVG.
func GenerateStoryCoverSVG(ctx context.Context, cfg *config.Config, title, summary, content string) (string, string) {
	if cfg == nil || strings.TrimSpace(cfg.GeminiAPIKey) == "" {
		return BuildFallbackCoverSVG(title), "fallback"
	}

	plainContent := strings.TrimSpace(htmlTagRegex.ReplaceAllString(content, " "))
	plainContent = strings.Join(strings.Fields(plainContent), " ")
	if len(plainContent) > 900 {
		plainContent = plainContent[:900]
	}

	prompt := fmt.Sprintf(`Generate a clean, minimalist unDraw-style flat-vector SVG illustration for a journal story.
Requirements:
- Output ONLY raw valid <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 W H" width="W" height="H" fill="none">...</svg> with no markdown fences or explanation.
- Tightly crop the viewBox around the drawn artwork so there is at most 20px of empty space outside the drawn elements on every side.
- Transparent background (do not add a full-canvas background rect).
- Use this exact cohesive color palette: dark charcoal/navy #2F2E41, slate #3F3D56, emerald/teal #00BFA6 and #009688, coral-pink #FF6584, warm peach #FFB8B8, soft neutrals #F2F2F2 and #E6E6E6.
- Design a unique composition, perspective, and visual metaphor tailored specifically to the story below (avoid generic repetitive scenes).
- Do NOT include <script>, <foreignObject>, external links, or text elements.

Story Title: %s
Summary: %s
Excerpt: %s`, title, summary, plainContent)

	model := cfg.GeminiModel
	if strings.TrimSpace(model) == "" {
		model = "gemini-2.5-flash"
	}

	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", url.PathEscape(model))
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.7,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return BuildFallbackCoverSVG(title), "fallback"
	}

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return BuildFallbackCoverSVG(title), "fallback"
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", cfg.GeminiAPIKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return BuildFallbackCoverSVG(title), "fallback"
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BuildFallbackCoverSVG(title), "fallback"
	}

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return BuildFallbackCoverSVG(title), "fallback"
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return BuildFallbackCoverSVG(title), "fallback"
	}

	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return BuildFallbackCoverSVG(title), "fallback"
	}

	sanitized, err := SanitizeSVG(parsed.Candidates[0].Content.Parts[0].Text)
	if err != nil {
		return BuildFallbackCoverSVG(title), "fallback"
	}

	return sanitized, "gemini"
}

// UploadCoverSVG uploads an SVG string to Firebase Cloud Storage under blogs/ and returns its publicUrl and fileName.
func UploadCoverSVG(ctx context.Context, cfg *config.Config, fbClient *fb.Client, title, svgContent string) (string, string, error) {
	if fbClient == nil || fbClient.BucketHandle == nil {
		return "", "", fmt.Errorf("storage bucket handle not available")
	}

	slug := slugCleanRegex.ReplaceAllString(strings.ToLower(strings.TrimSpace(title)), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "story"
	}
	if len(slug) > 40 {
		slug = slug[:40]
	}

	fileName := fmt.Sprintf("cover_%s_%d.svg", slug, time.Now().UnixMilli())
	refPath := sanitizeStoragePath("blogs", fileName)

	obj := fbClient.BucketHandle.Object(refPath)
	writer := obj.NewWriter(ctx)
	writer.ContentType = "image/svg+xml"

	if _, err := io.WriteString(writer, svgContent); err != nil {
		_ = writer.Close()
		return "", "", fmt.Errorf("failed to write SVG to storage: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", "", fmt.Errorf("failed to finalize SVG upload: %w", err)
	}

	encodedPath := url.PathEscape(refPath)
	publicURL := fmt.Sprintf("https://firebasestorage.googleapis.com/v0/b/%s/o/%s?alt=media",
		cfg.FirebaseStorageBucket, encodedPath)

	return publicURL, fileName, nil
}
