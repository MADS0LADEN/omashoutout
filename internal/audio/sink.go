package audio

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const SinkName = "shoutout"

type Sink struct{ module string }
type SinkState struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Muted  bool   `json:"mute"`
	Volume map[string]struct {
		Value uint32 `json:"value"`
	} `json:"volume"`
}

func (s SinkState) VolumePercent() float64 {
	var v uint32
	for _, c := range s.Volume {
		v = max(v, c.Value)
	}
	return float64(v) * 100 / 65536
}
func pactl(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "pactl", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pactl %s: %w: %s", args[0], err, strings.TrimSpace(string(b)))
	}
	return b, nil
}
func ReadSink(ctx context.Context) (SinkState, error) {
	b, err := pactl(ctx, "--format=json", "list", "sinks")
	if err != nil {
		return SinkState{}, err
	}
	var sinks []SinkState
	if err = json.Unmarshal(b, &sinks); err != nil {
		return SinkState{}, err
	}
	for _, s := range sinks {
		if s.Name == SinkName {
			return s, nil
		}
	}
	return SinkState{}, errors.New("ShoutOut output is unavailable")
}
func SetMuted(ctx context.Context, muted bool) error {
	_, err := pactl(ctx, "set-sink-mute", SinkName, strconv.FormatBool(muted))
	return err
}
func NewSink(ctx context.Context) (*Sink, error) {
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
			return &Sink{module: strconv.Itoa(m.Index)}, nil
		}
	}
	b, err = pactl(ctx, "load-module", "module-null-sink", "sink_name="+SinkName, "rate=48000", "channels=2", "format=float32le", `sink_properties=device.description="ShoutOut" device.icon_name="audio-speakers" shoutout.owner=shoutout priority.session=1`)
	if err != nil {
		return nil, err
	}
	s := &Sink{module: strings.TrimSpace(string(b))}
	if _, err = strconv.ParseUint(s.module, 10, 32); err != nil {
		return nil, err
	}
	// Only a newly-created device starts muted. Existing native desktop controls
	// survive daemon restarts; there is no separate application-level mute.
	if err = SetMuted(ctx, true); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (s *Sink) Close() error {
	_, err := pactl(context.Background(), "unload-module", s.module)
	return err
}

// RemoveSink is used by uninstall, not ordinary service restarts. Keeping the
// device prevents applications jumping to loud hardware during reconnection.
func RemoveSink(ctx context.Context) error {
	b, err := pactl(ctx, "--format=json", "list", "modules")
	if err != nil {
		return err
	}
	var modules []struct {
		Index    int    `json:"index"`
		Name     string `json:"name"`
		Argument string `json:"argument"`
	}
	if err = json.Unmarshal(b, &modules); err != nil {
		return err
	}
	for _, m := range modules {
		if m.Name == "module-null-sink" && strings.Contains(m.Argument, "shoutout.owner=shoutout") {
			if _, err = pactl(ctx, "unload-module", strconv.Itoa(m.Index)); err != nil {
				return err
			}
		}
	}
	return nil
}

// WatchSink follows the mute and volume state the desktop shows.
func WatchSink(ctx context.Context) (<-chan SinkState, error) {
	cmd := exec.CommandContext(ctx, "pactl", "subscribe")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	states := make(chan SinkState, 1)
	go func() {
		defer close(states)
		defer cmd.Wait()
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.Contains(line, " on sink ") {
				continue
			}
			state, err := ReadSink(ctx)
			if err != nil {
				continue
			}
			select {
			case states <- state:
			default:
				select {
				case <-states:
				default:
				}
				select {
				case states <- state:
				default:
				}
			}
		}
	}()
	return states, nil
}
