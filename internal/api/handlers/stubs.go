package handlers

import "net/http"

func stubJSON(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"status": "not_implemented"})
}
