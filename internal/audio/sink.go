package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const SinkName = "shoutout"

type Sink struct{ module string }

func pactl(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "pactl", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pactl %s: %w: %s", args[0], err, strings.TrimSpace(string(b)))
	}
	return b, nil
}
func NewSink(ctx context.Context) (*Sink, error) {
	// Reconcile only modules carrying this application's ownership property.
	b, err := pactl(ctx, "--format=json", "list", "modules")
	if err != nil {
		return nil, err
	}
	var modules []struct {
		Index    int    `json:"index"`
		Name     string `json:"name"`
		Argument string `json:"argument"`
	}
	if err = json.Unmarshal(b, &modules); err != nil {
		return nil, err
	}
	for _, m := range modules {
		if m.Name == "module-null-sink" && strings.Contains(m.Argument, "shoutout.owner=shoutout") {
			if _, err = pactl(ctx, "unload-module", strconv.Itoa(m.Index)); err != nil {
				return nil, err
			}
		}
	}
	b, err = pactl(ctx, "load-module", "module-null-sink", "sink_name="+SinkName, "rate=48000", "channels=2", "format=float32le", `sink_properties=device.description="Shoutout" device.icon_name="audio-speakers" shoutout.owner=shoutout priority.session=1`)
	if err != nil {
		return nil, err
	}
	s := &Sink{module: strings.TrimSpace(string(b))}
	if _, err = strconv.ParseUint(s.module, 10, 32); err != nil {
		return nil, fmt.Errorf("invalid module ID: %w", err)
	}
	// A new output starts at a useful desktop level; PCM attenuation is independent.
	if _, err = pactl(ctx, "set-sink-volume", SinkName, "100%"); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (s *Sink) Close() error {
	_, err := pactl(context.Background(), "unload-module", s.module)
	return err
}
