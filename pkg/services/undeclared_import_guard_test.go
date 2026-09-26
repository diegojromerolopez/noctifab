package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUndeclaredImportGuard(t *testing.T) {
	guard := NewUndeclaredImportGuard("", nil)

	t.Run("Python standard library imports pass without manifest", func(t *testing.T) {
		code := `
import os
import sys
from typing import Dict, List
from pathlib import Path
import unittest
`
		violations := guard.ValidateFileImports("src/app.py", code, nil)
		assert.Empty(t, violations)
	})

	t.Run("Python relative imports pass without manifest", func(t *testing.T) {
		code := `
from . import helper
from ..models import User
`
		violations := guard.ValidateFileImports("src/views.py", code, nil)
		assert.Empty(t, violations)
	})

	t.Run("Python undeclared third-party import is flagged", func(t *testing.T) {
		code := `
import os
import requests
from bs4 import BeautifulSoup
`
		violations := guard.ValidateFileImports("src/scraper.py", code, []string{"pytest"})
		require.Len(t, violations, 2)
		assert.Equal(t, "requests", violations[0].Import)
		assert.Equal(t, 3, violations[0].Line)
		assert.Equal(t, "bs4", violations[1].Import)
		assert.Equal(t, 4, violations[1].Line)
		assert.Contains(t, violations[0].Error(), "undeclared import violation")
	})

	t.Run("Python third-party import declared in manifest passes", func(t *testing.T) {
		code := `
import requests
import redis
`
		violations := guard.ValidateFileImports("src/client.py", code, []string{"requests>=2.28.0", "redis==4.5.1"})
		assert.Empty(t, violations)
	})

	t.Run("Node standard library imports pass without manifest", func(t *testing.T) {
		code := `
import fs from "fs";
const http = require("http");
`
		violations := guard.ValidateFileImports("src/server.ts", code, nil)
		assert.Empty(t, violations)
	})

	t.Run("Node undeclared third-party package is flagged", func(t *testing.T) {
		code := `
import express from "express";
`
		violations := guard.ValidateFileImports("src/index.js", code, nil)
		require.Len(t, violations, 1)
		assert.Equal(t, "express", violations[0].Import)
	})

	t.Run("Rust standard crates pass and undeclared external crate is flagged", func(t *testing.T) {
		code := `
use std::sync::Arc;
use core::fmt;
use serde::Serialize;
`
		violations := guard.ValidateFileImports("src/main.rs", code, nil)
		require.Len(t, violations, 1)
		assert.Equal(t, "serde", violations[0].Import)
		assert.Equal(t, 4, violations[0].Line)
	})
}
