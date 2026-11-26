// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetupPyNameAndVersion checks that setup.py yields only the name
// and version that the first setup() call passes as plain string
// literals, never a value from a comment, a string, a nested call or
// an assignment elsewhere in the file.
func TestSetupPyNameAndVersion(t *testing.T) {
	tests := []struct {
		name, setupPy, wantName, wantVersion string
	}{
		{
			name:        "literal name and version",
			setupPy:     "from setuptools import setup\n\nsetup(\n    name='plain-pkg',\n    version=\"3.1.4\",\n)\n",
			wantName:    "plain-pkg",
			wantVersion: "3.1.4",
		},
		{
			name: "name passed through an upper-case variable beside a decoy",
			setupPy: "from setuptools import setup\n\npackage_name = \"decoy-name\"\nNAME = \"var-name\"\n\n" +
				"setup(name=NAME, version=\"1.0.0\")\n",
			wantVersion: "1.0.0",
		},
		{
			name: "keyword of a nested Extension call",
			setupPy: "from setuptools import Extension, setup\n\n" +
				"ext = Extension(name=\"real_pkg._speedups\", sources=[\"a.c\"])\n\n" +
				"setup(name=\"real-pkg\", version=\"1.0.0\",\n" +
				"      ext_modules=[Extension(name=\"real_pkg._other\", sources=[\"b.c\"])])\n",
			wantName:    "real-pkg",
			wantVersion: "1.0.0",
		},
		{
			name:        "commented-out call above the real one",
			setupPy:     "from setuptools import setup\n\n# setup(name=\"old-name\", version=\"0.1\")\nsetup(name=\"new-name\", version=\"2.0.0\")\n",
			wantName:    "new-name",
			wantVersion: "2.0.0",
		},
		{
			name:        "version-like assignment above the call",
			setupPy:     "from setuptools import setup\n\nmin_python_version = \"3.9\"\nsetup(name=\"ver-decoy\", version=\"1.0.0\")\n",
			wantName:    "ver-decoy",
			wantVersion: "1.0.0",
		},
		{
			name:        "name passed through a lower-case variable",
			setupPy:     "from setuptools import setup\n\nname = \"lower-var\"\nsetup(name=name, version=\"1.0.0\")\n",
			wantVersion: "1.0.0",
		},
		{
			name:        "call inside a docstring",
			setupPy:     "\"\"\"Build with:\n\n    setup(name=\"doc-name\", version=\"9.9\")\n\"\"\"\nimport setuptools\n\nsetuptools.setup(name='doc-real', version='0.2.0')\n",
			wantName:    "doc-real",
			wantVersion: "0.2.0",
		},
		{
			name:        "def setup above the real call",
			setupPy:     "import setuptools\n\ndef setup(name=\"def-name\", version=\"0.0.1\"):\n    pass\n\nsetuptools.setup(name=\"def-real\", version=\"1.1.0\")\n",
			wantName:    "def-real",
			wantVersion: "1.1.0",
		},
		{
			name: "imported setup above a parenthesised assignment",
			setupPy: "from setuptools import setup\n" +
				"(NAME, VERSION) = (\"vars-pkg\", \"0.1\")\n" +
				"setup(name=\"real-pkg\", version=\"1.2.3\")\n",
			wantName:    "real-pkg",
			wantVersion: "1.2.3",
		},
		{
			name:        "backslash continuation before the call's parenthesis",
			setupPy:     "from setuptools import setup\n\nsetup \\\n    (name=\"cont-pkg\", version=\"1.0\")\n",
			wantName:    "cont-pkg",
			wantVersion: "1.0",
		},
		{
			name: "call in a string nested in an f-string field",
			setupPy: "from setuptools import setup\n\n" +
				"note = f\"{ \"setup(name='decoy', version='9.0')\" }\"\n" +
				"spec = f'{{setup}} {x:{w}} {d[\"k\"]!r:>{w}} {f\"{ \"(\" }\"}'\n" +
				"setup(name='real', version='1.0')\n",
			wantName:    "real",
			wantVersion: "1.0",
		},
		{
			name: "lambda defaults named name and version",
			setupPy: "from setuptools import setup\n\n" +
				"setup(name=NAME, version=\"1.0\", callback=lambda x, name=\"decoy\", y=None: x,\n" +
				"      hook=lambda a, version=\"9.9\", b=lambda c, name=\"inner\": c: a)\n",
			wantVersion: "1.0",
		},
		{
			name: "positional lambda before the keywords",
			setupPy: "from setuptools import setup\n\n" +
				"setup(lambda p, version=\"9.9\", q=0: p, name=\"lam-pkg\", version=\"2.0\")\n",
			wantName:    "lam-pkg",
			wantVersion: "2.0",
		},
		{
			name: "string literals holding a backslash give no value",
			setupPy: "from setuptools import setup\n\n" +
				"setup(name=r\"raw\\pkg\", version=u\"1.\\x30\")\n",
		},
		{
			name: "escaped newline in a version gives no value",
			setupPy: "from setuptools import setup\n\n" +
				"setup(name=\"nl-pkg\", version=\"1.\\\n0\")\n",
			wantName: "nl-pkg",
		},
		{
			name: "backslash before a replacement field in an f-string",
			setupPy: "from setuptools import setup\n\n" +
				"note = f\"\\{ \"setup(name='decoy', version='9.0')\" }\"\n" +
				"setup(name='real', version='1.0')\n",
			wantName:    "real",
			wantVersion: "1.0",
		},
		{
			name: "backslash before a replacement field in a raw f-string",
			setupPy: "from setuptools import setup\n\n" +
				"note = rf\"\\{ \"setup(name='decoy', version='9.0')\" }\"\n" +
				"setup(name='real', version='1.0')\n",
			wantName:    "real",
			wantVersion: "1.0",
		},
		{
			name: "unterminated f-string gives no value",
			setupPy: "from setuptools import setup\n\n" +
				"setup(name='real', version='1.0')\n" +
				"note = f\"{ x\n",
		},
		{
			name: "unclosed replacement field gives no value",
			setupPy: "from setuptools import setup\n\n" +
				"note = f'''{x:>10'''\n" +
				"setup(name='real', version='1.0')\n",
		},
		{
			name: "class pattern in a case clause above the real call",
			setupPy: "from setuptools import setup\n\n" +
				"match command:\n" +
				"    case setup(name=\"decoy\", version=\"9.0\"):\n" +
				"        pass\n" +
				"setup(name=\"real\", version=\"1.0\")\n",
			wantName:    "real",
			wantVersion: "1.0",
		},
		{
			name: "star pattern in a case clause above the real call",
			setupPy: "from setuptools import setup\n\n" +
				"match command:\n" +
				"    case *rest, setup(name=\"decoy\", version=\"9.0\"):\n" +
				"        pass\n" +
				"setup(name=\"real\", version=\"1.0\")\n",
			wantName:    "real",
			wantVersion: "1.0",
		},
		{
			name: "or pattern from a dot-leading number above the real call",
			setupPy: "from setuptools import setup\n\n" +
				"match command:\n" +
				"    case .5 | setup(name=\"decoy\", version=\"9.0\"):\n" +
				"        pass\n" +
				"setup(name=\"real\", version=\"1.0\")\n",
			wantName:    "real",
			wantVersion: "1.0",
		},
		{
			name: "call in the body of a case clause",
			setupPy: "from setuptools import setup\n\n" +
				"match command:\n" +
				"    case \"build\":\n" +
				"        setup(name=\"real\", version=\"1.0\")\n",
			wantName:    "real",
			wantVersion: "1.0",
		},
		{
			name: "call indented under a main guard",
			setupPy: "import setuptools\n\n" +
				"if __name__ == \"__main__\":\n" +
				"    setuptools.setup(name=\"real\", version=\"1.0\")\n",
			wantName:    "real",
			wantVersion: "1.0",
		},
		{
			name:    "call on the right of an assignment gives no value",
			setupPy: "from setuptools import setup\n\ndist = setup(name=\"real\", version=\"1.0\")\n",
		},
		{
			name:    "call after a colon on one line gives no value",
			setupPy: "from setuptools import setup\n\nif x: setup(name=\"real\", version=\"1.0\")\n",
		},
		{
			name:    "f-string and concatenation give no value",
			setupPy: "from setuptools import setup\n\nsetup(name=f\"{BASE}-pkg\", version=\"1.\" \"0\")\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := createTempProject(t, map[string]string{"setup.py": tt.setupPy})
			metadata, err := NewExtractor().Extract(dir)
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, metadata.Name, "name")
			assert.Equal(t, tt.wantVersion, metadata.Version, "version")
		})
	}
}
