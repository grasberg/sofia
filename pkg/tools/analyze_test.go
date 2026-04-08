package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeTool_Name(t *testing.T) {
	tool := NewAnalyzeTool()
	assert.Equal(t, "analyze", tool.Name())
}

func TestAnalyzeTool_Symbols_Go(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "sample.go")

	src := `package sample

import "fmt"

// Greeter defines a greeting interface.
type Greeter interface {
	Greet(name string) string
}

// Person is a struct that implements Greeter.
type Person struct {
	Name string
	Age  int
}

func (p *Person) Greet(name string) string {
	return fmt.Sprintf("Hello %s, I'm %s", name, p.Name)
}

func NewPerson(name string, age int) *Person {
	return &Person{Name: name, Age: age}
}

type Status int
`
	require.NoError(t, os.WriteFile(goFile, []byte(src), 0o644))

	tool := NewAnalyzeTool()
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{
		"action": "symbols",
		"path":   goFile,
	})

	assert.False(t, result.IsError, "expected no error, got: %s", result.ForLLM)
	assert.Contains(t, result.ForLLM, "Package: sample")
	assert.Contains(t, result.ForLLM, "interface Greeter")
	assert.Contains(t, result.ForLLM, "struct Person")
	assert.Contains(t, result.ForLLM, "method (*Person) Greet")
	assert.Contains(t, result.ForLLM, "func NewPerson")
	assert.Contains(t, result.ForLLM, "type Status")
}

func TestAnalyzeTool_Symbols_Python(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "app.py")

	src := `import os
from pathlib import Path

class UserService:
    def __init__(self):
        pass

    def get_user(self, user_id):
        pass

async def fetch_data(url):
    pass

def main():
    svc = UserService()
`
	require.NoError(t, os.WriteFile(pyFile, []byte(src), 0o644))

	tool := NewAnalyzeTool()
	result := tool.Execute(context.Background(), map[string]any{
		"action": "symbols",
		"path":   pyFile,
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "class UserService")
	assert.Contains(t, result.ForLLM, "func __init__")
	assert.Contains(t, result.ForLLM, "func get_user")
	assert.Contains(t, result.ForLLM, "func fetch_data")
	assert.Contains(t, result.ForLLM, "func main")
}

func TestAnalyzeTool_Symbols_JS(t *testing.T) {
	dir := t.TempDir()
	jsFile := filepath.Join(dir, "app.js")

	src := `export class Router {
  constructor() {}
}

export async function handleRequest(req) {
  return req;
}

const processData = async (data) => {
  return data;
}
`
	require.NoError(t, os.WriteFile(jsFile, []byte(src), 0o644))

	tool := NewAnalyzeTool()
	result := tool.Execute(context.Background(), map[string]any{
		"action": "symbols",
		"path":   jsFile,
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "class Router")
	assert.Contains(t, result.ForLLM, "func handleRequest")
	assert.Contains(t, result.ForLLM, "func processData")
}

func TestAnalyzeTool_Symbols_Rust(t *testing.T) {
	dir := t.TempDir()
	rsFile := filepath.Join(dir, "lib.rs")

	src := `pub struct Config {
    name: String,
}

pub trait Handler {
    fn handle(&self);
}

impl Config {
    pub fn new(name: &str) -> Self {
        Config { name: name.to_string() }
    }
}

fn helper() {}
`
	require.NoError(t, os.WriteFile(rsFile, []byte(src), 0o644))

	tool := NewAnalyzeTool()
	result := tool.Execute(context.Background(), map[string]any{
		"action": "symbols",
		"path":   rsFile,
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "struct Config")
	assert.Contains(t, result.ForLLM, "trait Handler")
	assert.Contains(t, result.ForLLM, "impl Config")
	assert.Contains(t, result.ForLLM, "func new")
	assert.Contains(t, result.ForLLM, "func helper")
}

func TestAnalyzeTool_Dependencies_Go(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")

	src := `package main

import (
	"fmt"
	"os"

	mylog "github.com/example/log"
)

func main() {
	fmt.Println(os.Args)
	mylog.Info("done")
}
`
	require.NoError(t, os.WriteFile(goFile, []byte(src), 0o644))

	tool := NewAnalyzeTool()
	result := tool.Execute(context.Background(), map[string]any{
		"action": "dependencies",
		"path":   goFile,
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "fmt")
	assert.Contains(t, result.ForLLM, "os")
	assert.Contains(t, result.ForLLM, "mylog github.com/example/log")
}

func TestAnalyzeTool_Dependencies_Python(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "app.py")

	src := `import os
import sys
from pathlib import Path
from collections import OrderedDict
`
	require.NoError(t, os.WriteFile(pyFile, []byte(src), 0o644))

	tool := NewAnalyzeTool()
	result := tool.Execute(context.Background(), map[string]any{
		"action": "dependencies",
		"path":   pyFile,
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "os")
	assert.Contains(t, result.ForLLM, "sys")
	assert.Contains(t, result.ForLLM, "pathlib")
	assert.Contains(t, result.ForLLM, "collections")
}

func TestAnalyzeTool_Dependencies_JS(t *testing.T) {
	dir := t.TempDir()
	jsFile := filepath.Join(dir, "index.js")

	src := `import express from 'express';
import { Router } from './router';
const fs = require('fs');
`
	require.NoError(t, os.WriteFile(jsFile, []byte(src), 0o644))

	tool := NewAnalyzeTool()
	result := tool.Execute(context.Background(), map[string]any{
		"action": "dependencies",
		"path":   jsFile,
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "express")
	assert.Contains(t, result.ForLLM, "./router")
	assert.Contains(t, result.ForLLM, "fs")
}

func TestAnalyzeTool_Overview(t *testing.T) {
	dir := t.TempDir()

	// Create a small project structure
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("package src\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "pkg", "util.go"), []byte("package pkg\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Project\n"), 0o644))

	tool := NewAnalyzeTool()
	result := tool.Execute(context.Background(), map[string]any{
		"action": "overview",
		"path":   dir,
		"depth":  float64(3),
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Directory:")
	assert.Contains(t, result.ForLLM, "main.go")
	assert.Contains(t, result.ForLLM, "src/")
	assert.Contains(t, result.ForLLM, "app.go")
	assert.Contains(t, result.ForLLM, "go")
	assert.Contains(t, result.ForLLM, "Total LOC:")
}

func TestAnalyzeTool_Overview_SkipsHidden(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "visible.go"), []byte("package main\n"), 0o644))

	tool := NewAnalyzeTool()
	result := tool.Execute(context.Background(), map[string]any{
		"action": "overview",
		"path":   dir,
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "visible.go")
	assert.NotContains(t, result.ForLLM, ".git")
}

func TestAnalyzeTool_Outline_Go(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "server.go")

	src := `package server

import "net/http"

type Server struct {
	addr string
}

type Handler interface {
	Handle(w http.ResponseWriter, r *http.Request)
}

func NewServer(addr string) *Server {
	return &Server{addr: addr}
}

func (s *Server) Start() error {
	return nil
}

func (s *Server) Stop() error {
	return nil
}
`
	require.NoError(t, os.WriteFile(goFile, []byte(src), 0o644))

	tool := NewAnalyzeTool()
	result := tool.Execute(context.Background(), map[string]any{
		"action": "outline",
		"path":   goFile,
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "package server")
	assert.Contains(t, result.ForLLM, "struct Server")
	assert.Contains(t, result.ForLLM, "interface Handler")
	assert.Contains(t, result.ForLLM, "func NewServer")
	assert.Contains(t, result.ForLLM, ".Start")
	assert.Contains(t, result.ForLLM, ".Stop")
}

func TestAnalyzeTool_Validation(t *testing.T) {
	tool := NewAnalyzeTool()
	ctx := context.Background()

	t.Run("missing action", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"path": "/tmp"})
		assert.True(t, result.IsError)
	})

	t.Run("missing path", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "symbols"})
		assert.True(t, result.IsError)
	})

	t.Run("unknown action", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "foobar", "path": "/tmp"})
		assert.True(t, result.IsError)
	})

	t.Run("overview on file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "file.go")
		require.NoError(t, os.WriteFile(f, []byte("package main"), 0o644))
		result := tool.Execute(ctx, map[string]any{"action": "overview", "path": f})
		assert.True(t, result.IsError)
	})

	t.Run("symbols on directory", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "symbols", "path": t.TempDir()})
		assert.True(t, result.IsError)
	})

	t.Run("nonexistent path", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "symbols", "path": "/nonexistent/path.go"})
		assert.True(t, result.IsError)
	})
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		hint string
		want string
	}{
		{"main.go", "", "go"},
		{"app.py", "", "python"},
		{"index.js", "", "js"},
		{"index.ts", "", "ts"},
		{"lib.rs", "", "rust"},
		{"app.rb", "", "ruby"},
		{"Main.java", "", "java"},
		{"file.txt", "", ""},
		{"file.txt", "python", "python"},
		{"main.go", "rust", "rust"}, // hint overrides
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectLanguage(tt.path, tt.hint)
			assert.Equal(t, tt.want, got)
		})
	}
}
