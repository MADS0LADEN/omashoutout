package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/lkarlslund/shoutout/internal/config"
)

func TestDecodeSingleJSONObject(t *testing.T) {
	def := config.Default()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(def); err != nil {
		t.Fatal(err)
	}
	var got config.Config
	if err := decodeSingleJSONObject(&buf, &got); err != nil {
		t.Fatal(err)
	}
	if got.Codec != def.Codec || got.Preset != def.Preset {
		t.Fatalf("got %+v, want default codec/preset", got)
	}
}

func TestDecodeSingleJSONObjectTrailing(t *testing.T) {
	def := config.Default()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(def); err != nil {
		t.Fatal(err)
	}
	if err := enc.Encode(def); err != nil {
		t.Fatal(err)
	}
	var got config.Config
	err := decodeSingleJSONObject(&buf, &got)
	if err == nil || !strings.Contains(err.Error(), "unexpected trailing data") {
		t.Fatalf("want unexpected trailing data, got %v", err)
	}
}

func TestDecodeSingleJSONObjectEmpty(t *testing.T) {
	var got config.Config
	err := decodeSingleJSONObject(strings.NewReader(""), &got)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if err != io.EOF && !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("want EOF, got %v", err)
	}
}
