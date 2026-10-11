package main

// This file holds the WebKitGTK rendering-fallback decision on its own, with no
// build tag and no cgo, so it can be unit-tested on any platform. The Linux
// glue that feeds it real inputs lives in app_linux.go.

// DMABUF fallback reasons, returned by decideDMABUFDisable.
const (
	dmabufReasonEnv      = "env"             // the user already set the variable
	dmabufReasonMarker   = "marker"          // remembered from an earlier run
	dmabufReasonNoRender = "no-render-node"  // nothing for DMABUF to hand buffers to
	dmabufReasonDefault  = "default-enabled" // leave the default path alone
)

// decideDMABUFDisable decides whether the DMABUF renderer should be turned off
// for this launch.
//
// WebKitGTK 2.42+ composites through the DMABUF renderer by default. On some
// drivers that path paints the first frame and then stops, so the window goes
// blank immediately after the QR code appears (issue #63, Intel HD Graphics 400
// / Braswell Gen-8). WEBKIT_DISABLE_DMABUF_RENDERER=1 falls back to a
// software-composited path that keeps painting.
//
// Inputs are passed in rather than read from the environment so the decision is
// testable, and so the caller owns every side effect.
//
//   - envValue: the current value of WEBKIT_DISABLE_DMABUF_RENDERER ("" if unset).
//   - markerExists: whether a previous run recorded the fallback.
//   - renderNodes: the DRM render nodes that exist, e.g. /dev/dri/renderD128.
//
// Returns disable=true when the caller must set the variable itself;
// persistMarker=true when the caller should record it for later launches.
func decideDMABUFDisable(envValue string, markerExists bool, renderNodes []string) (disable bool, persistMarker bool, reason string) {
	// 1. The user asked for it. Honour the variable, and remember it so the
	//    workaround survives a launch that does not come from a shell -- a
	//    desktop-entry click, which is how most people start the app. Without
	//    this, following the README's one-line fix works exactly once.
	if envValue != "" {
		return false, !markerExists, dmabufReasonEnv
	}
	// 2. A previous run recorded it.
	if markerExists {
		return true, false, dmabufReasonMarker
	}
	// 3. No render node at all. The DMABUF renderer has no device to hand
	//    buffers to, so it cannot be the correct path. True on VMs, containers
	//    and remote desktops, and it needs no table of specific hardware.
	if len(renderNodes) == 0 {
		return true, false, dmabufReasonNoRender
	}
	// Hardware that can composite keeps its hardware compositing.
	return false, false, dmabufReasonDefault
}
