// Copyright (c) 2026 Justin Andrew Wood. All rights reserved.
// This software is licensed under the AGPL-3.0.
// Commercial licensing is available at echosh-labs.com.
/*
File: internal/server/poller.go
Description: Background routines for Axis Mundi. Runs the AUTO-mode workspace refresh
ticker and the periodic system telemetry digest flusher.
*/
package server

import (
	"context"
	"time"
)

func (s *Server) bufferTelemetry(msg string) {
	select {
	case s.telemetryBuffer <- msg:
	default:
		s.logger.Warn("telemetry buffer full, dropping message")
	}
}

// runTelemetryFlusher periodically batches telemetry events and sends them via Chat API.
func (s *Server) runTelemetryFlusher(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var batch []string

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.telemetryBuffer:
			batch = append(batch, msg)
		case <-ticker.C:
			if len(batch) > 0 {
				digest := "🔔 *System Telemetry Digest*\n"
				for _, m := range batch {
					digest += "- " + m + "\n"
				}
				if s.user != nil && s.ws != nil {
					err := s.ws.SendDirectMessage(s.user.Email, digest)
					if err != nil {
						s.logger.Error("failed to send telemetry dm", "error", err)
					}
				}
				batch = nil // clear batch
			}
		}
	}
}

// runPoller processes periodic refreshes for AUTO mode.
func (s *Server) runPoller(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	remaining := autoRefreshTicks
	for {
		select {
		case <-ticker.C:
			s.modeMu.RLock()
			mode := s.mode
			s.modeMu.RUnlock()

			if mode == "AUTO" {
				remaining--
				s.broadcastTick(remaining)
				if remaining <= 0 {
					s.refreshRegistryCache()
					s.broadcastRegistry()
					remaining = autoRefreshTicks
				}
			} else {
				remaining = autoRefreshTicks
			}
		case <-ctx.Done():
			return
		}
	}
}
