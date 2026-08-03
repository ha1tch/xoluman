// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package web embeds xoluman's static assets (currently just the vendored
// modal controller) into the binary, matching the same go:embed pattern
// Seam AMS uses for its own web package — a single deployable binary
// with no dependency on files existing relative to the working directory.
package web

import "embed"

//go:embed static
var Static embed.FS
