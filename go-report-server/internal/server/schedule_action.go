package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/auth"
	"github.com/acugis/acugis-report-server-go/internal/schedules"
)

func (s *Server) handleScheduleAction(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if err := s.requirePlatformAdmin(r, user); err != nil {
		s.writeAuthzJSON(w, err)
		return
	}

	if r.Method != http.MethodPost {
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"success": false,
			"message": "Method not allowed.",
		})
		return
	}

	repo := s.deps.Schedules
	if repo == nil || !repo.Configured() {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"message": "Application database unavailable.",
		})
		return
	}

	if err := parsePostedForm(r); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "Failed to parse form.",
		})
		return
	}

	post := formToMap(r)

	if r.FormValue("delete") != "" {
		s.handleScheduleDelete(w, r, repo, post)
		return
	}
	if r.FormValue("run_now") != "" {
		s.handleScheduleRunNow(w, r, post)
		return
	}
	if r.FormValue("update") != "" {
		s.handleScheduleUpdate(w, r, repo, post)
		return
	}
	if r.FormValue("create") != "" {
		s.handleScheduleCreate(w, r, repo, post)
		return
	}

	s.writeJSON(w, http.StatusBadRequest, map[string]any{
		"success": false,
		"message": "Error while processing your request.",
	})
}

func (s *Server) handleScheduleDelete(w http.ResponseWriter, r *http.Request, repo *schedules.Repository, post map[string]any) {
	id, _ := strconv.Atoi(strings.TrimSpace(strFromMap(post, "id")))
	if id <= 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Invalid schedule id."})
		return
	}

	if _, err := repo.Get(r.Context(), id); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": scheduleActionError(err)})
		return
	}

	if s.deps.ScheduleSys != nil {
		_ = s.deps.ScheduleSys.Remove(r.Context(), id)
	}

	if err := repo.Delete(r.Context(), id); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": "Schedule delete failed.",
		})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("Schedule %d deleted.", id),
	})
}

func (s *Server) handleScheduleRunNow(w http.ResponseWriter, r *http.Request, post map[string]any) {
	id, _ := strconv.Atoi(strings.TrimSpace(strFromMap(post, "id")))
	if id <= 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Invalid schedule id."})
		return
	}

	if s.deps.Schedules != nil {
		if _, err := s.deps.Schedules.Get(r.Context(), id); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": scheduleActionError(err)})
			return
		}
	}

	if s.deps.ScheduleRun == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"message": "Schedule runner unavailable.",
		})
		return
	}

	run := s.deps.ScheduleRun.Execute(r.Context(), id, "manual")
	if !run.OK {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"success":      false,
			"message":      run.Message,
			"execution_id": run.ExecutionID,
		})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"message":      "Report generated: " + run.OutputFile,
		"execution_id": run.ExecutionID,
		"output_file":  run.OutputFile,
		"email_status": run.EmailStatus,
	})
}

func (s *Server) handleScheduleCreate(w http.ResponseWriter, r *http.Request, repo *schedules.Repository, post map[string]any) {
	noemail := schedules.BoolValue(post["noemail"], false)
	if !noemail && strings.TrimSpace(strFromMap(post, "email")) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "Email recipients are required.",
		})
		return
	}

	reportsChecker, dsChecker := s.scheduleCheckers()
	normalized := schedules.NormalizePost(r.Context(), post, reportsChecker, dsChecker)
	if !normalized.OK {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": normalized.Err,
		})
		return
	}

	data := normalized.Data
	applyNoEmailFromPost(data, post)

	id, err := repo.Create(r.Context(), *data)
	if err != nil {
		msg := scheduleActionError(err)
		if msg == "Error while processing your request." {
			msg = "Schedule record create failed."
		}
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": msg})
		return
	}

	saved, err := repo.Get(r.Context(), id)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": "Schedule created but reload failed.",
			"id":      id,
		})
		return
	}

	if syncMsg := s.syncScheduleSystemd(r, id, saved); syncMsg != "" {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"message": syncMsg,
			"id":      id,
		})
		return
	}

	if strings.ToLower(strings.TrimSpace(saved.CronPeriod)) == "now" {
		s.runScheduleAsync(id)
		s.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": fmt.Sprintf("Schedule %d created and started.", id),
			"id":      id,
		})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("Schedule %d created.", id),
		"id":      id,
	})
}

func (s *Server) handleScheduleUpdate(w http.ResponseWriter, r *http.Request, repo *schedules.Repository, post map[string]any) {
	id, _ := strconv.Atoi(strings.TrimSpace(strFromMap(post, "id")))
	if id <= 0 {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Invalid schedule id."})
		return
	}

	noemail := schedules.BoolValue(post["noemail"], false)
	if !noemail && strings.TrimSpace(strFromMap(post, "email")) == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "Email recipients are required.",
		})
		return
	}

	reportsChecker, dsChecker := s.scheduleCheckers()
	normalized := schedules.NormalizePost(r.Context(), post, reportsChecker, dsChecker)
	if !normalized.OK {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": normalized.Err,
		})
		return
	}

	data := normalized.Data
	applyNoEmailFromPost(data, post)

	if _, err := repo.Get(r.Context(), id); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": scheduleActionError(err)})
		return
	}

	if err := repo.Update(r.Context(), id, *data); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": "Schedule record update failed.",
		})
		return
	}

	saved, err := repo.Get(r.Context(), id)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": "Schedule saved but reload failed.",
		})
		return
	}

	if syncMsg := s.syncScheduleSystemd(r, id, saved); syncMsg != "" {
		s.writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": syncMsg})
		return
	}

	if strings.ToLower(strings.TrimSpace(saved.CronPeriod)) == "now" {
		s.runScheduleAsync(id)
		s.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": fmt.Sprintf("Schedule %d updated and started.", id),
		})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("Schedule %d updated.", id),
	})
}

func (s *Server) syncScheduleSystemd(r *http.Request, id int, saved *schedules.Schedule) string {
	if s.deps.ScheduleSys == nil || saved == nil {
		return ""
	}
	sync := s.deps.ScheduleSys.SyncAfterSave(r.Context(), id, saved)
	if sync.OK {
		return ""
	}
	detail := strings.TrimSpace(sync.Error)
	var message string
	if sync.TimerRequired {
		message = fmt.Sprintf("Schedule %d saved but systemd timer install failed. The previous timer remains active when one existed.", id)
	} else {
		message = fmt.Sprintf("Schedule %d saved but environment file write failed.", id)
	}
	if detail != "" {
		message += " " + detail
	} else {
		message += " Check schedules directory permissions and server error log."
	}
	return message
}
