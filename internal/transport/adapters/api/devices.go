package api

import (
	"net/http"

	"live-semantic/internal/application/session"
	"live-semantic/internal/implementation/streamer/input"

	"github.com/gin-gonic/gin"
)

// maxProbeIndex bounds how many device indices GET /api/v1/devices
// probes (see input.ProbeDevices's own doc comment on why this is
// index-probing, not a true OS enumeration). Arbitrary but generous for
// a dev machine (built-in + a couple of USB cameras) — each extra index
// costs one real open/close syscall round-trip per request.
const maxProbeIndex = 5

// devicesController exposes best-effort local camera device discovery —
// added 2026-08-13 directly in response to a real problem: the GUI used
// to offer a vague "caméra serveur" vs "caméra navigateur" choice with no
// way to see which physical device would actually be used, so two
// sources could (and did) silently fight over the same webcam. This
// replaces that with an actual device list, each entry flagged busy if a
// running session already claims it — the GUI greys those out instead of
// letting the user hit the same silent conflict again.
type devicesController struct {
	manager *session.Manager
}

func newDevicesController(manager *session.Manager) *devicesController {
	return &devicesController{manager: manager}
}

type deviceInfo struct {
	Index int  `json:"index"`
	Busy  bool `json:"busy"`
}

// list handles GET /api/v1/devices. Busy state comes from
// session.Manager's own bookkeeping (running "local" sessions), not a
// fresh probe — see input.ProbeDevices's skipIndices parameter.
func (dc *devicesController) list(c *gin.Context) {
	busy := map[int]bool{}
	for _, s := range dc.manager.List() {
		if s.Source.Kind == "local" && s.Running {
			busy[s.Source.Device] = true
		}
	}

	indices := input.ProbeDevices(maxProbeIndex, busy)
	devices := make([]deviceInfo, 0, len(indices))
	for _, idx := range indices {
		devices = append(devices, deviceInfo{Index: idx, Busy: busy[idx]})
	}
	c.JSON(http.StatusOK, gin.H{"devices": devices})
}
