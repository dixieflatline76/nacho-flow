// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package data

import "embed"

//go:embed agents/*.json shell.json reasoning.json
var CatalogFS embed.FS

