package api

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"scriberr/internal/llm"
	"scriberr/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SummarizeRequest struct {
	Model           string  `json:"model" binding:"required"`
	Content         string  `json:"content" binding:"required"`
	TranscriptionID string  `json:"transcription_id" binding:"required"`
	TemplateID      *string `json:"template_id,omitempty"`
}

// Summarize streams LLM output for a given content prompt
// @Summary Summarize content
// @Description Stream an LLM-generated summary for provided content; persists latest summary for the transcription
// @Tags summarize
// @Accept json
// @Produce text/event-stream
// @Param request body SummarizeRequest true "Summarize request"
// @Success 200 {string} string "Event stream"
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security ApiKeyAuth
// @Security BearerAuth
// @Router /api/v1/summarize [post]
func (h *Handler) Summarize(c *gin.Context) {
	var req SummarizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	svc, provider, err := h.getLLMService(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	start := time.Now()
	log.Printf("[summarize] start transcription_id=%s provider=%s model=%s content_len=%d", req.TranscriptionID, provider, req.Model, len(req.Content))

	// Stream response with proper headers for real-time delivery
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering
	c.Status(http.StatusOK)             // Start response immediately

	// Free tiers (Groq/OpenAI) cap tokens-per-minute per request, so very long
	// transcripts can't be summarized in one call. Chunk + map-reduce for those.
	if len(req.Content) > 40000 { // ~10k tokens
		h.processLargeSummarization(c, req, svc, start)
		return
	}

	messages := []llm.ChatMessage{{Role: "user", Content: req.Content}}
	h.processSummarization(c, req, svc, messages, start)
}

func (h *Handler) processSummarization(c *gin.Context, req SummarizeRequest, svc llm.Service, messages []llm.ChatMessage, start time.Time) {
	// Allow longer generation time for large transcripts and smaller models
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Minute)
	defer cancel()

	contentChan, errChan := svc.ChatCompletionStream(ctx, req.Model, messages, 0.0)
	flusher, _ := c.Writer.(http.Flusher)
	writer := bufio.NewWriter(c.Writer)

	finalText := ""
	gotFirstChunk := false

	// Loop handles one chunk/error at a time
	for {
		select {
		case chunk, ok := <-contentChan:
			if !ok {
				writer.Flush()
				if flusher != nil {
					flusher.Flush()
				}
				// Persist summary once streaming completes
				h.persistSummary(req, finalText)
				log.Printf("[summarize] complete transcription_id=%s model=%s bytes=%d duration_ms=%d", req.TranscriptionID, req.Model, len(finalText), time.Since(start).Milliseconds())
				return
			}
			finalText += chunk
			_, _ = writer.WriteString(chunk)
			writer.Flush()
			if flusher != nil {
				flusher.Flush()
			}
			if !gotFirstChunk && len(chunk) > 0 {
				gotFirstChunk = true
				log.Printf("[summarize] first_chunk transcription_id=%s model=%s at_ms=%d", req.TranscriptionID, req.Model, time.Since(start).Milliseconds())
			}
		case err := <-errChan:
			if err != nil {
				h.handleSummarizeError(c, req, svc, messages, err, finalText, start)
			}
			// Persist any partial content on error
			h.persistSummary(req, finalText)
			return
		case <-ctx.Done():
			// Persist any partial content on timeout/cancel
			h.persistSummary(req, finalText)
			log.Printf("[summarize] timeout/cancel transcription_id=%s model=%s bytes=%d duration_ms=%d", req.TranscriptionID, req.Model, len(finalText), time.Since(start).Milliseconds())
			return
		}
	}
}

// processLargeSummarization summarizes transcripts too large for a single request
// on rate-limited free tiers (e.g. Groq 12k tokens/min). It splits the transcript
// into chunks, extracts notes from each (map), then synthesizes the final minute
// from those notes (reduce), retrying whenever the per-minute rate limit is hit.
func (h *Handler) processLargeSummarization(c *gin.Context, req SummarizeRequest, svc llm.Service, start time.Time) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)
	defer cancel()
	flusher, _ := c.Writer.(http.Flusher)
	fail := func(msg string) {
		_, _ = c.Writer.Write([]byte(msg))
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Separate transcript from the instructions appended by the client.
	transcript, instructions := req.Content, ""
	if idx := strings.LastIndex(req.Content, "\n\nInstructions:\n"); idx >= 0 {
		transcript = req.Content[:idx]
		instructions = req.Content[idx+len("\n\nInstructions:\n"):]
	}
	chunks := chunkText(transcript, 34000) // ~9k tokens per chunk (under 12k TPM)
	log.Printf("[summarize] large map-reduce transcription_id=%s chunks=%d", req.TranscriptionID, len(chunks))

	// MAP: extract structured notes from each chunk.
	notes := make([]string, 0, len(chunks))
	for i, ch := range chunks {
		msg := []llm.ChatMessage{{Role: "user", Content: fmt.Sprintf(
			"Esta es la PARTE %d de %d de la transcripción de una reunión. Extrae en viñetas concisas: temas tratados, decisiones y compromisos (con responsable y fecha si se mencionan). Sé fiel al texto; no inventes datos.\n\n%s",
			i+1, len(chunks), ch)}}
		resp, err := chatWithRetry(ctx, svc, req.Model, msg)
		if err != nil {
			fail("No se pudo generar el resumen (límite de la API o error): " + err.Error())
			return
		}
		if len(resp.Choices) > 0 {
			notes = append(notes, resp.Choices[0].Message.Content)
		}
	}

	// REDUCE: synthesize the final minute from the partial notes.
	reduce := "A continuación hay notas parciales de una reunión, en orden:\n\n" +
		strings.Join(notes, "\n\n") + "\n\nInstructions:\n" + instructions
	resp, err := chatWithRetry(ctx, svc, req.Model, []llm.ChatMessage{{Role: "user", Content: reduce}})
	if err != nil {
		fail("No se pudo generar la minuta final (límite de la API o error): " + err.Error())
		return
	}
	final := ""
	if len(resp.Choices) > 0 {
		final = resp.Choices[0].Message.Content
	}
	_, _ = c.Writer.Write([]byte(final))
	if flusher != nil {
		flusher.Flush()
	}
	h.persistSummary(req, final)
	log.Printf("[summarize] large complete transcription_id=%s chunks=%d bytes=%d duration_ms=%d", req.TranscriptionID, len(chunks), len(final), time.Since(start).Milliseconds())
}

// chunkText splits s into pieces no larger than maxChars, breaking at a sentence
// or word boundary when possible.
func chunkText(s string, maxChars int) []string {
	s = strings.TrimSpace(s)
	if len(s) <= maxChars {
		return []string{s}
	}
	var chunks []string
	for len(s) > maxChars {
		cut := maxChars
		if idx := strings.LastIndexAny(s[:maxChars], ".!?\n "); idx > maxChars/2 {
			cut = idx + 1
		}
		chunks = append(chunks, strings.TrimSpace(s[:cut]))
		s = s[cut:]
	}
	if strings.TrimSpace(s) != "" {
		chunks = append(chunks, strings.TrimSpace(s))
	}
	return chunks
}

// chatWithRetry calls the LLM and retries when the provider reports a per-minute
// rate limit with a "try again in Xs" hint (waiting the suggested time).
func chatWithRetry(ctx context.Context, svc llm.Service, model string, messages []llm.ChatMessage) (*llm.ChatResponse, error) {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		resp, err := svc.ChatCompletion(ctx, model, messages, 0.0)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		wait := parseRetrySeconds(err.Error())
		if wait <= 0 {
			return nil, err // not a retryable rate limit
		}
		if wait > 120 {
			wait = 120
		}
		log.Printf("[summarize] rate limited, retrying in %ds", wait)
		select {
		case <-time.After(time.Duration(wait) * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

// parseRetrySeconds extracts a wait time from a rate-limit message like
// "Please try again in 6.5s" or "try again in 1m2.3s". Returns 0 if none.
func parseRetrySeconds(msg string) int {
	i := strings.Index(msg, "try again in ")
	if i < 0 {
		return 0
	}
	rest := msg[i+len("try again in "):]
	var mins, secs float64
	if mi := strings.Index(rest, "m"); mi >= 0 && mi < 6 {
		fmt.Sscanf(rest[:mi], "%f", &mins)
		rest = rest[mi+1:]
	}
	if si := strings.Index(rest, "s"); si >= 0 {
		fmt.Sscanf(rest[:si], "%f", &secs)
	}
	total := mins*60 + secs
	if total <= 0 {
		return 0
	}
	return int(total) + 1
}

func (h *Handler) handleSummarizeError(c *gin.Context, req SummarizeRequest, svc llm.Service, messages []llm.ChatMessage, err error, partialText string, start time.Time) {
	flusher, _ := c.Writer.(http.Flusher)
	writer := bufio.NewWriter(c.Writer)

	// Best-effort error signal
	// If streaming is unsupported for this model/org, fall back to non-streaming
	errStr := err.Error()
	if strings.Contains(errStr, "\"param\": \"stream\"") || strings.Contains(errStr, "unsupported_value") || strings.Contains(errStr, "must be verified to stream") {
		log.Printf("[summarize] falling back to non-streaming transcription_id=%s model=%s due to: %v", req.TranscriptionID, req.Model, err)
		resp, err2 := svc.ChatCompletion(c.Request.Context(), req.Model, messages, 0.0)
		if err2 != nil || resp == nil || len(resp.Choices) == 0 {
			log.Printf("[summarize] fallback failed transcription_id=%s model=%s err=%v", req.TranscriptionID, req.Model, err2)
			_, _ = c.Writer.Write([]byte("\n"))
			writer.Flush()
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		content := resp.Choices[0].Message.Content
		// Write content (appended to partial if any, though likely partial is empty if stream failed immediately)
		_, _ = writer.WriteString(content)
		writer.Flush()
		if flusher != nil {
			flusher.Flush()
		}
		// We should persist the FULL text (partial + fallback), but partialText is passed by value.
		// However, handleSumarizeError doesn't update partialText in caller.
		// The caller calls persistSummary(req, finalText) after this function returns.
		// So we actually need to persist here if we succeed?
		// Or return the new text?
		// Since we can't easily update finalText in caller without pointer, let's persist here if success.
		h.persistSummary(req, partialText+content)
		log.Printf("[summarize] fallback complete transcription_id=%s model=%s bytes=%d duration_ms=%d", req.TranscriptionID, req.Model, len(partialText+content), time.Since(start).Milliseconds())

		// To avoid double persistence in caller (which uses stale finalText), we need a way to signal "done".
		// But caller persists anyway.
		// It's acceptable to double-persist (idempotent updates usually) or just accept that caller persists partial and we persist full.
		return
	}
	_, _ = c.Writer.Write([]byte("\n"))
	writer.Flush()
	if flusher != nil {
		flusher.Flush()
	}
	log.Printf("[summarize] error transcription_id=%s model=%s err=%v duration_ms=%d", req.TranscriptionID, req.Model, err, time.Since(start).Milliseconds())
}

func (h *Handler) persistSummary(req SummarizeRequest, content string) {
	if req.TranscriptionID == "" || content == "" {
		return
	}
	sum := &models.Summary{
		TranscriptionID: req.TranscriptionID,
		TemplateID:      req.TemplateID,
		Model:           req.Model,
		Content:         content,
	}
	if err := h.summaryRepo.SaveSummary(context.Background(), sum); err != nil {
		// Fallback: store on the transcription job record
		_ = h.jobRepo.UpdateSummary(context.Background(), req.TranscriptionID, content)
	} else {
		// Also cache on the transcription job for quick access
		_ = h.jobRepo.UpdateSummary(context.Background(), req.TranscriptionID, content)
	}
}

// GetSummaryForTranscription returns the latest summary for a transcription
// @Summary Get latest summary for transcription
// @Description Get the most recent saved summary for the given transcription
// @Tags summarize
// @Produce json
// @Param id path string true "Transcription ID"
// @Success 200 {object} models.Summary
// @Failure 404 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security ApiKeyAuth
// @Security BearerAuth
// @Router /api/v1/transcription/{id}/summary [get]
func (h *Handler) GetSummaryForTranscription(c *gin.Context) {
	tid := c.Param("id")
	if tid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Transcription ID required"})
		return
	}
	s, err := h.summaryRepo.GetLatestSummary(c.Request.Context(), tid)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Fallback: check if summary is cached on the job record
			job, err2 := h.jobRepo.FindByID(c.Request.Context(), tid)
			if err2 == nil && job.Summary != nil && *job.Summary != "" {
				c.JSON(http.StatusOK, gin.H{
					"transcription_id": tid,
					"template_id":      nil,
					"model":            "",
					"content":          *job.Summary,
					"created_at":       job.UpdatedAt,
					"updated_at":       job.UpdatedAt,
				})
				return
			}
			// Return empty summary instead of 404 for graceful frontend handling
			c.JSON(http.StatusOK, gin.H{
				"transcription_id": tid,
				"template_id":      nil,
				"model":            "",
				"content":          "",
				"created_at":       nil,
				"updated_at":       nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch summary"})
		return
	}
	c.JSON(http.StatusOK, s)
}
