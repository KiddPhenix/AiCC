package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"aihelper/internal/app/capture"
	"aihelper/internal/domain"
	"aihelper/internal/service"
)

type Handler struct {
	service             *service.SolutionService
	importService       *service.ImportService
	captureService      *service.CaptureService
	analysisService     *service.CaptureAnalysisService
	analysisServiceFunc func() *service.CaptureAnalysisService
	settingsService     *service.SettingsService
}

func NewHandler(solutionService *service.SolutionService, importService *service.ImportService, captureService *service.CaptureService, analysisService *service.CaptureAnalysisService, settingsService *service.SettingsService) *Handler {
	return &Handler{
		service:             solutionService,
		importService:       importService,
		captureService:      captureService,
		analysisService:     analysisService,
		settingsService:     settingsService,
		analysisServiceFunc: func() *service.CaptureAnalysisService { return analysisService },
	}
}

func (h *Handler) SetAnalysisServiceFactory(factory func() *service.CaptureAnalysisService) {
	if factory == nil {
		return
	}
	h.analysisServiceFunc = factory
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/solutions", h.handleSolutions)
	mux.HandleFunc("/api/v1/solutions/", h.handleSolutionActions)
	mux.HandleFunc("/api/v1/imports", h.handleImports)
	mux.HandleFunc("/api/v1/settings", h.handleSettings)
	mux.HandleFunc("/api/v1/captures", h.handleCaptures)
	mux.HandleFunc("/api/v1/captures/", h.handleCaptureActions)
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join("web", "static"))))
	return withRequestLogging(withCORS(mux))
}

func (h *Handler) handleSolutions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createSolution(w, r)
	case http.MethodGet:
		h.listSolutions(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleSolutionActions(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/solutions/"), "/"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid solution id"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		h.updateSolution(w, r, id)
	case http.MethodDelete:
		h.deleteSolution(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) createSolution(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var input domain.CreateSolutionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid json",
		})
		return
	}

	solution, err := h.service.CreateSolution(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidSolution):
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "title, solution_text, efficacy_source, status are required and must be valid",
			})
		case errors.Is(err, domain.ErrDuplicate):
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":    "duplicate solution",
				"existing": solution,
			})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": "internal server error",
			})
		}
		return
	}

	writeJSON(w, http.StatusCreated, solution)
}

func (h *Handler) updateSolution(w http.ResponseWriter, r *http.Request, id int64) {
	defer r.Body.Close()
	var input domain.UpdateSolutionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	solution, err := h.service.UpdateSolution(r.Context(), id, input)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidSolution):
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "title, solution_text, status are required and must be valid"})
		case errors.Is(err, domain.ErrDuplicate):
			writeJSON(w, http.StatusConflict, map[string]any{"error": "duplicate solution", "existing": solution})
		case errors.Is(err, domain.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "solution not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		}
		return
	}
	writeJSON(w, http.StatusOK, solution)
}

func (h *Handler) deleteSolution(w http.ResponseWriter, r *http.Request, id int64) {
	if err := h.service.DeleteSolution(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "solution not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) listSolutions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))

	filter := domain.SolutionFilter{
		Query:          strings.TrimSpace(query.Get("q")),
		Status:         strings.TrimSpace(strings.ToLower(query.Get("status"))),
		EfficacySource: strings.TrimSpace(strings.ToLower(query.Get("efficacy_source"))),
		Tag:            strings.TrimSpace(strings.ToLower(query.Get("tag"))),
		Limit:          limit,
		Offset:         offset,
	}

	solutions, err := h.service.ListSolutions(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "internal server error",
		})
		return
	}
	total, err := h.service.CountSolutions(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "internal server error",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": solutions,
		"count": len(solutions),
		"total": total,
	})
}

func (h *Handler) handleImports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var input service.ImportInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid json",
		})
		return
	}

	result, err := h.importService.Import(r.Context(), input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleCaptures(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listCaptures(w, r)
	case http.MethodDelete:
		h.clearUnanalyzedCaptures(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) listCaptures(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))
	var analyzedOnly *bool
	if raw := strings.TrimSpace(strings.ToLower(query.Get("analyzed"))); raw != "" {
		value := raw == "true" || raw == "1"
		analyzedOnly = &value
	}

	items, err := h.captureService.ListCaptures(r.Context(), capture.Filter{
		Query:        strings.TrimSpace(query.Get("q")),
		Limit:        limit,
		Offset:       offset,
		AnalyzedOnly: analyzedOnly,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "internal server error",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (h *Handler) clearUnanalyzedCaptures(w http.ResponseWriter, r *http.Request) {
	deleted, err := h.captureService.DeleteUnanalyzedCaptures(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

func (h *Handler) handleCaptureActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/captures/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodDelete {
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid capture id"})
			return
		}
		h.deleteCapture(w, r, id)
		return
	}
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid capture id"})
		return
	}

	switch {
	case parts[1] == "analyze" && r.Method == http.MethodPost:
		h.analyzeCapture(w, r, id)
	case parts[1] == "promote" && r.Method == http.MethodPost:
		h.promoteCapture(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) deleteCapture(w http.ResponseWriter, r *http.Request, captureID int64) {
	if err := h.captureService.DeleteCapture(r.Context(), captureID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) analyzeCapture(w http.ResponseWriter, r *http.Request, captureID int64) {
	insight, err := h.getAnalysisService().AnalyzeCapture(r.Context(), captureID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAnalysisUnavailable):
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "AI analysis unavailable. Set OPENAI_API_KEY first."})
		case errors.Is(err, service.ErrAnalysisTimedOut):
			writeJSON(w, http.StatusGatewayTimeout, map[string]any{"error": "AI analysis timed out after 15 seconds"})
		case errors.Is(err, service.ErrAnalysisRecentlyRequested):
			writeJSON(w, http.StatusConflict, map[string]any{"error": "recently sent the same content for analysis, please wait"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, insight)
}

func (h *Handler) promoteCapture(w http.ResponseWriter, r *http.Request, captureID int64) {
	solution, err := h.getAnalysisService().PromoteCapture(r.Context(), captureID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAnalysisUnavailable):
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "analyze this capture first"})
		case errors.Is(err, domain.ErrDuplicate):
			writeJSON(w, http.StatusConflict, map[string]any{"error": "duplicate solution", "existing": solution})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusCreated, solution)
}

func (h *Handler) getAnalysisService() *service.CaptureAnalysisService {
	if h.analysisServiceFunc != nil {
		return h.analysisServiceFunc()
	}
	return h.analysisService
}

func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := h.settingsService.Get()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		defer r.Body.Close()
		var input service.SettingsDTO
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		settings, err := h.settingsService.Update(input)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		next.ServeHTTP(recorder, r)
		log.Printf("http %s %s -> %d (%s)", r.Method, r.URL.RequestURI(), recorder.status, time.Since(start).Truncate(time.Millisecond))
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
