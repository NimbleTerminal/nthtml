// Copyright (c) 2026 Neomantra Corp. All rights reserved.
// SPDX-License-Identifier: AGPL-3.0-only WITH LicenseRef-NimbleTerminal-Linking-Exception OR LicenseRef-Neomantra-Commercial

package nthtml

import (
	"sync"

	"github.com/charmbracelet/x/ansi/kitty"
	"nimbleterminal.dev/ntpicture"
)

var placeholderCells = sync.OnceValue(func() int {
	// The Kitty graphics protocol addresses each placeholder cell by a row and
	// a column diacritic from a fixed table; a cell past the table cannot be
	// told apart from the first one, and ntpicture refuses such a placement.
	for i := 1; i < 1<<16; i++ {
		if kitty.Diacritic(i) == kitty.Diacritic(0) {
			return i
		}
	}
	return 1 << 16
})

// MaxKittyPlaceholderCells is the widest and tallest placement, in cells, one
// Kitty Unicode-placeholder image can span. A host-painted viewer presents at
// most this many columns; a host that lays pages out at the terminal's width
// should size the viewer no wider, so the band is painted once at the size the
// transport can show and the columns to the right stay blank instead of the
// whole page.
func MaxKittyPlaceholderCells() int { return placeholderCells() }

// kittyCols is the number of columns the picture can present: the model's
// width, bounded by the placeholder grid when the transport is Kitty.
func (m *Model) kittyCols() int {
	if m.pic.Mode() == ntpicture.PictureKitty {
		return max(1, min(m.cols, MaxKittyPlaceholderCells()))
	}
	return m.cols
}
