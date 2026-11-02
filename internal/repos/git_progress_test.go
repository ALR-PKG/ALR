// ALR - Any Linux Repository
// Copyright (C) 2026 The ALR Authors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package repos

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitProgressEnumeratingUsesSpinner(t *testing.T) {
	var out bytes.Buffer
	progress := newGitProgressWriter(&out)

	for _, fragment := range []string{"Enumerating ob", "jects: 202", "98, done.\r"} {
		n, err := progress.Write([]byte(fragment))
		require.NoError(t, err)
		assert.Equal(t, len(fragment), n)
	}
	require.NoError(t, progress.Close())

	assert.Contains(t, out.String(), "⠋ Enumerating objects: 20298")
	assert.NotContains(t, out.String(), "%")
	assert.NotContains(t, out.String(), "done")
}

func TestGitProgressKnownTotalUsesBar(t *testing.T) {
	var out bytes.Buffer
	progress := newGitProgressWriter(&out)

	_, err := progress.Write([]byte("Counting objects: 50% (10/20)\r"))
	require.NoError(t, err)
	require.NoError(t, progress.Close())

	assert.Contains(t, out.String(), "Counting objects")
	assert.Contains(t, out.String(), strings.Repeat("●", 20)+strings.Repeat("○", 20))
	assert.Contains(t, out.String(), "50% (10/20)")
}

func TestGitProgressRecognizesServerStages(t *testing.T) {
	for _, stage := range []string{
		"Compressing objects",
		"Receiving objects",
		"Resolving deltas",
	} {
		t.Run(stage, func(t *testing.T) {
			var out bytes.Buffer
			progress := newGitProgressWriter(&out)

			_, err := progress.Write([]byte(stage + ": 25% (1/4)\r"))
			require.NoError(t, err)
			require.NoError(t, progress.Close())

			assert.Contains(t, out.String(), stage)
			assert.Contains(t, out.String(), "25% (1/4)")
			assert.Contains(t, out.String(), "●")
		})
	}
}

func TestGitProgressSuppressesUnknownServerText(t *testing.T) {
	var out bytes.Buffer
	progress := newGitProgressWriter(&out)

	_, err := progress.Write([]byte("remote: English-only server text\n"))
	require.NoError(t, err)
	require.NoError(t, progress.Close())
	assert.Empty(t, out.String())
}

func TestGitProgressAcceptsRemotePrefix(t *testing.T) {
	for _, test := range []struct {
		message string
		want    string
	}{
		{message: "remote: Enumerating objects: 20, done.\r", want: "Enumerating objects: 20"},
		{message: "remote: Receiving objects: 50% (1/2)\r", want: "50% (1/2)"},
	} {
		var out bytes.Buffer
		progress := newGitProgressWriter(&out)

		_, err := progress.Write([]byte(test.message))
		require.NoError(t, err)
		require.NoError(t, progress.Close())
		assert.Contains(t, out.String(), test.want)
	}
}

func TestGitProgressCloseIsIdempotent(t *testing.T) {
	var out bytes.Buffer
	progress := newGitProgressWriter(&out)

	require.NoError(t, progress.Close())
	require.NoError(t, progress.Close())
	assert.Empty(t, out.String())
}
