// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The expected list matches what cargo 1.99 reports for the fixture:
// cargo metadata --no-deps lists the same members, with publish = []
// for beta, delta and gamma.
func TestPublishablePackagesInTheWorkspaceFixture(t *testing.T) {
	got, warnings := publishablePackages(filepath.Join("testdata", "workspace", "Cargo.toml"))
	want := []PublishablePackage{
		{Name: "ws-root", Version: "0.9.0", ManifestPath: "Cargo.toml"},
		{Name: "alpha", Version: "1.2.3", ManifestPath: "crates/alpha/Cargo.toml", Registries: []string{"internal"}},
		{Name: "implicit", Version: "0.4.0", ManifestPath: "implicit/Cargo.toml"},
		{Name: "shared", Version: "0.3.0", ManifestPath: "libs/shared/Cargo.toml", Registries: []string{"crates-io", "internal"}},
		{Name: "kept", Version: "0.1.0", ManifestPath: "tools/kept/Cargo.toml"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings %q", warnings)
	}
}

func TestPublishablePackagesOutputForm(t *testing.T) {
	metadata, err := NewExtractor().Extract(filepath.Join("testdata", "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(metadata.LanguageSpecific["publishable_packages"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(encoded), `[{"name":"ws-root","version":"0.9.0","manifest_path":"Cargo.toml"},{"name":"alpha"`) {
		t.Errorf("unexpected encoding %s", encoded)
	}
	if count := metadata.LanguageSpecific["publishable_package_count"]; count != 5 {
		t.Errorf("count = %v, want 5", count)
	}

	none := extractManifest(t, "[package]\nname = \"private\"\nversion = \"1.0.0\"\npublish = false\n")
	if encoded, _ := json.Marshal(none["publishable_packages"]); string(encoded) != "[]" {
		t.Errorf("no publishable packages encoded as %s, want []", encoded)
	}
}

func writeManifests(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(name)), content)
	}
	return root
}

func names(packages []PublishablePackage) []string {
	result := []string{}
	for _, pkg := range packages {
		result = append(result, pkg.Name)
	}
	return result
}

func TestPublishablePackages(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		want     []string
		warnings int
	}{
		{
			name: "single package ignores path dependencies",
			files: map[string]string{
				"Cargo.toml":     "[package]\nname = \"solo\"\nversion = \"1.0.0\"\n[dependencies]\ndep = { path = \"dep\" }\n",
				"dep/Cargo.toml": "[package]\nname = \"dep\"\nversion = \"1.0.0\"\n",
			},
			want: []string{"solo"},
		},
		{
			name: "virtual manifest",
			files: map[string]string{
				"Cargo.toml":   "[workspace]\nmembers = [\"a\", \"missing\", \"empty\"]\n",
				"a/Cargo.toml": "[package]\nname = \"a\"\nversion = \"1.0.0\"\n",
				"empty/x.txt":  "not a crate",
			},
			want: []string{"a"},
		},
		{
			name: "path dependency outside the root and a cycle",
			files: map[string]string{
				"ws/Cargo.toml":   "[workspace]\nmembers = [\"a\", \"b\"]\n",
				"ws/a/Cargo.toml": "[package]\nname = \"a\"\nversion = \"1.0.0\"\n[dependencies]\nb = { path = \"../b\" }\nout = { path = \"../../out\" }\n",
				"ws/b/Cargo.toml": "[package]\nname = \"b\"\nversion = \"1.0.0\"\n[dependencies]\na = { path = \"../a\" }\n",
				"out/Cargo.toml":  "[package]\nname = \"out\"\nversion = \"1.0.0\"\n",
			},
			want: []string{"a", "b"},
		},
		{
			name: "untrusted values are left out",
			files: map[string]string{
				"Cargo.toml":        "[workspace]\nmembers = [\"*\"]\n",
				"name/Cargo.toml":   "[package]\nname = \"SECRET$(id)\"\nversion = \"1.0.0\"\n",
				"ver/Cargo.toml":    "[package]\nname = \"ver\"\nversion = \"1.0 SECRET\"\n",
				"reg/Cargo.toml":    "[package]\nname = \"reg\"\nversion = \"1.0.0\"\npublish = [\"SECRET reg\", \"ok\"]\n",
				"bad/Cargo.toml":    "[package]\nname = \"bad\"\nversion = \"1.0.0\"\npublish = \"SECRET\"\n",
				"sp ace/Cargo.toml": "[package]\nname = \"spaced\"\nversion = \"1.0.0\"\n",
			},
			want:     []string{"reg"},
			warnings: 5,
		},
		{
			name: "exclude entries are literal paths, as in cargo",
			files: map[string]string{
				"Cargo.toml":                  "[workspace]\nmembers = [\"crates/*\"]\nexclude = [\"crates/private-*\", \"crates/oth?r\", \"crates/gone\"]\n",
				"crates/private-a/Cargo.toml": "[package]\nname = \"private-a\"\nversion = \"1.0.0\"\n",
				"crates/other/Cargo.toml":     "[package]\nname = \"other\"\nversion = \"1.0.0\"\n",
				"crates/gone/Cargo.toml":      "[package]\nname = \"gone\"\nversion = \"1.0.0\"\n",
			},
			want: []string{"other", "private-a"},
		},
		{
			// A member selected below its workspace root: cargo would
			// resolve these from the parent, which the action never reads.
			name: "inherited version or publish the manifest cannot resolve",
			files: map[string]string{
				"Cargo.toml":    "[workspace]\nmembers = [\"v\", \"p\", \"ok\"]\n[workspace.package]\nrust-version = \"1.85\"\n",
				"v/Cargo.toml":  "[package]\nname = \"v\"\nversion.workspace = true\n",
				"p/Cargo.toml":  "[package]\nname = \"p\"\nversion = \"1.0.0\"\npublish.workspace = true\n",
				"ok/Cargo.toml": "[package]\nname = \"ok\"\nversion = \"1.0.0\"\n",
			},
			want:     []string{"ok"},
			warnings: 2,
		},
		{
			name: "invalid glob",
			files: map[string]string{
				"Cargo.toml": "[workspace]\nmembers = [\"[\"]\n",
			},
			want:     []string{},
			warnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeManifests(t, tt.files)
			manifest := filepath.Join(root, "Cargo.toml")
			if _, err := os.Stat(manifest); err != nil {
				manifest = filepath.Join(root, "ws", "Cargo.toml")
			}
			got, warnings := publishablePackages(manifest)
			if !reflect.DeepEqual(names(got), tt.want) {
				t.Errorf("got %v, want %v", names(got), tt.want)
			}
			if len(warnings) != tt.warnings {
				t.Errorf("got %d warnings %q, want %d", len(warnings), warnings, tt.warnings)
			}
			for _, warning := range warnings {
				if strings.Contains(warning, untrusted) {
					t.Errorf("warning repeats repository content: %q", warning)
				}
			}
			for _, pkg := range got {
				if strings.Contains(strings.Join(pkg.Registries, ","), untrusted) {
					t.Errorf("registry list carries repository content: %v", pkg.Registries)
				}
			}
		})
	}
}

// Every value below was checked with cargo 1.99 cargo metadata.
func TestParseCrateVersion(t *testing.T) {
	accepted := map[string]string{
		"1.0.0":                      "1.0.0",
		"0.0.0":                      "0.0.0",
		"1.2.3-alpha.1+build.5":      "1.2.3-alpha.1+build.5",
		"1.0.0-0.3.7":                "1.0.0-0.3.7",
		"1.0.0-x.7.z.92":             "1.0.0-x.7.z.92",
		"1.0.0+001":                  "1.0.0+001",
		"1.0.0-alpha-a.b-c":          "1.0.0-alpha-a.b-c",
		"1.0.0--":                    "1.0.0--",
		"1.0.0-0a":                   "1.0.0-0a",
		"18446744073709551615.0.0":   "18446744073709551615.0.0",
		"1.0.0-99999999999999999999": "1.0.0-99999999999999999999",
		" 1.0.0":                     "1.0.0",
		"\t1.0.0\n":                  "1.0.0",
	}
	for version, want := range accepted {
		if got, ok := parseCrateVersion(version); !ok || got != want {
			t.Errorf("parseCrateVersion(%q) = %q, %v; want %q, true", version, got, ok, want)
		}
	}
	rejected := []string{
		"", "banana", "1..0", "1.0", "1", "v1.0.0", "1.0.0.0",
		"01.0.0", "1.00.0", "1.0.00", "1.0.0-01",
		"18446744073709551616.0.0",
		"1.0.0-", "1.0.0+", "1.0.0-a..b", "1.0.0+a..b", "1.0.0-alpha+",
		"1.0.0-a_b", "1.0.0+a_b", "1.0.0+build+x", "1.0.0-\u03b1",
		"1.0.0 -alpha", "1 .0.0", "+1.0.0", "-1.0.0",
	}
	for _, version := range rejected {
		if got, ok := parseCrateVersion(version); ok {
			t.Errorf("parseCrateVersion(%q) = %q, true; cargo rejects it", version, got)
		}
	}
}

func TestPublishableVersionsCargoRejects(t *testing.T) {
	root := writeManifests(t, map[string]string{
		"Cargo.toml":        "[workspace]\nmembers = [\"*\"]\n",
		"banana/Cargo.toml": "[package]\nname = \"banana\"\nversion = \"banana\"\n",
		"dots/Cargo.toml":   "[package]\nname = \"dots\"\nversion = \"1..0\"\n",
		"zero/Cargo.toml":   "[package]\nname = \"zero\"\nversion = \"01.0.0\"\n",
		"pad/Cargo.toml":    "[package]\nname = \"pad\"\nversion = \" 1.2.3-rc.1 \"\n",
	})
	got, warnings := publishablePackages(filepath.Join(root, "Cargo.toml"))
	want := []PublishablePackage{{Name: "pad", Version: "1.2.3-rc.1", ManifestPath: "pad/Cargo.toml"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if len(warnings) != 3 {
		t.Errorf("got %d warnings %q, want 3", len(warnings), warnings)
	}
	for _, warning := range warnings {
		if !strings.Contains(warning, "version cargo rejects") {
			t.Errorf("unexpected warning %q", warning)
		}
	}
}

// cargo 1.99 rejects a package or registry name starting with a digit
// or "-", and accepts one starting with "_".
func TestPublishableNamesNeedCargosFirstCharacter(t *testing.T) {
	root := writeManifests(t, map[string]string{
		"Cargo.toml":       "[workspace]\nmembers = [\"*\"]\n",
		"digit/Cargo.toml": "[package]\nname = \"1demo\"\nversion = \"1.0.0\"\n",
		"dash/Cargo.toml":  "[package]\nname = \"-demo\"\nversion = \"1.0.0\"\n",
		"under/Cargo.toml": "[package]\nname = \"_demo\"\nversion = \"1.0.0\"\npublish = [\"1reg\", \"-reg\", \"_reg\"]\n",
	})
	got, warnings := publishablePackages(filepath.Join(root, "Cargo.toml"))
	want := []PublishablePackage{{Name: "_demo", Version: "1.0.0", ManifestPath: "under/Cargo.toml", Registries: []string{"_reg"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if len(warnings) != 3 {
		t.Errorf("got %d warnings %q, want 3", len(warnings), warnings)
	}
}

func TestPublishableMemberLinkingOutside(t *testing.T) {
	root := writeManifests(t, map[string]string{
		"outside/Cargo.toml":     "[package]\nname = \"outside\"\nversion = \"1.0.0\"\n",
		"ws/Cargo.toml":          "[workspace]\nmembers = [\"crates/*\"]\n",
		"ws/crates/a/Cargo.toml": "[package]\nname = \"a\"\nversion = \"1.0.0\"\n",
	})
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "ws", "crates", "link")); err != nil {
		t.Fatal(err)
	}
	got, warnings := publishablePackages(filepath.Join(root, "ws", "Cargo.toml"))
	if !reflect.DeepEqual(names(got), []string{"a"}) || len(warnings) != 1 ||
		!strings.Contains(warnings[0], "outside the workspace") {
		t.Errorf("got %v %q, want [a] and one warning", names(got), warnings)
	}
}

// Cargo accepts absolute dependency paths: one that normalises to a
// directory under the real workspace root is a member, like its relative
// form, and one outside the root is not.
func TestPublishableAbsolutePathDependencies(t *testing.T) {
	root := writeManifests(t, map[string]string{
		"ws/abs/Cargo.toml":    "[package]\nname = \"abs\"\nversion = \"1.0.0\"\n",
		"ws/dotted/Cargo.toml": "[package]\nname = \"dotted\"\nversion = \"1.0.0\"\n",
		"ws/shared/Cargo.toml": "[package]\nname = \"shared\"\nversion = \"1.0.0\"\n",
		"out/Cargo.toml":       "[package]\nname = \"out\"\nversion = \"1.0.0\"\n",
	})
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	ws := filepath.ToSlash(filepath.Join(realRoot, "ws"))
	writeFile(t, filepath.Join(root, "ws", "Cargo.toml"), fmt.Sprintf(
		"[workspace]\n[workspace.dependencies]\nshared = { path = %q }\n"+
			"[package]\nname = \"top\"\nversion = \"1.0.0\"\n"+
			"[dependencies]\nabs = { path = %q }\ndotted = { path = %q }\nout = { path = %q }\nshared = { workspace = true }\n",
		ws+"/shared", ws+"/abs", ws+"/abs/../dotted", ws+"/../out"))

	got, warnings := publishablePackages(filepath.Join(root, "ws", "Cargo.toml"))
	paths := []string{}
	for _, pkg := range got {
		paths = append(paths, pkg.ManifestPath)
	}
	want := []string{"Cargo.toml", "abs/Cargo.toml", "dotted/Cargo.toml", "shared/Cargo.toml"}
	if !reflect.DeepEqual(paths, want) || len(warnings) != 0 {
		t.Errorf("got %v %q, want %v and no warnings", paths, warnings, want)
	}
}

// Cargo (1.99) accepts a package outside the workspace root, listed in
// members or reached as a path dependency, when its package.workspace
// key leads back to the root; without that key, or naming another root,
// it is no member. Under GitHub Actions nothing outside the checkout is
// read.
func TestPublishableMembersOutsideTheRoot(t *testing.T) {
	pkg := func(name, workspace string) string {
		manifest := "[package]\nname = \"" + name + "\"\nversion = \"1.0.0\"\n"
		if workspace != "" {
			manifest += "workspace = \"" + workspace + "\"\n"
		}
		return manifest
	}
	root := writeManifests(t, map[string]string{
		"ws/Cargo.toml": "[workspace]\nmembers = [\"../shared\", \"../nopointer\", \"../wrong\", \"../tofile\"]\n" +
			"[package]\nname = \"top\"\nversion = \"1.0.0\"\n[dependencies]\ndep = { path = \"../dep\" }\n",
		"shared/Cargo.toml":    pkg("shared", "../ws/"),
		"dep/Cargo.toml":       pkg("dep", "../ws"),
		"nopointer/Cargo.toml": pkg("nopointer", ""),
		"wrong/Cargo.toml":     pkg("wrong", "../other"),
		"tofile/Cargo.toml":    pkg("tofile", "../ws/Cargo.toml"),
		"other/Cargo.toml":     "[workspace]\n",
	})
	manifest := filepath.Join(root, "ws", "Cargo.toml")

	for _, tt := range []struct {
		name      string
		workspace string
		want      []string
	}{
		{"inside the checkout", root, []string{"../dep/Cargo.toml", "../shared/Cargo.toml", "Cargo.toml"}},
		{"outside the checkout", filepath.Join(root, "ws"), []string{"Cargo.toml"}},
		{"outside GitHub Actions", "", []string{"../dep/Cargo.toml", "../shared/Cargo.toml", "Cargo.toml"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_WORKSPACE", tt.workspace)
			got, warnings := publishablePackages(manifest)
			paths := []string{}
			for _, p := range got {
				paths = append(paths, p.ManifestPath)
			}
			if !reflect.DeepEqual(paths, tt.want) || len(warnings) != 0 {
				t.Errorf("got %v %q, want %v and no warnings", paths, warnings, tt.want)
			}
		})
	}
}

// Each members entry expands as cargo 1.99 expanded it (through the glob
// crate) in the same layout: cargo metadata listed exactly these
// manifests, and rejected the invalid patterns.
func TestPublishableMembersGlobsMatchCargo(t *testing.T) {
	files := map[string]string{}
	for _, dir := range []string{"crates/.h", "crates/a", "crates/b", "crates/bz", "deep/b", "deep/q/b", "nest", "nest/x", "nest/x/y"} {
		name := strings.NewReplacer("/", "-", ".", "").Replace(dir)
		files[dir+"/Cargo.toml"] = "[package]\nname = \"" + name + "\"\nversion = \"1.0.0\"\n"
	}
	root := writeManifests(t, files)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	// Reach the workspace through a symlink, as /var reaches /private/var
	// on macOS: cargo matches an absolute entry against the real root.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		entry    string
		want     []string
		warnings int
	}{
		{"crates/*", []string{"crates/.h", "crates/a", "crates/b", "crates/bz"}, 0},
		{"crates/[!b]*", []string{"crates/.h", "crates/a"}, 0},
		{"crates/[^b]*", []string{"crates/b", "crates/bz"}, 0},
		{"crates/?", []string{"crates/a", "crates/b"}, 0},
		{"crates/[]a]", []string{"crates/a"}, 0},
		{"crates/[a-b]", []string{"crates/a", "crates/b"}, 0},
		{"nest/**", []string{"nest/x", "nest/x/y"}, 0},
		{"deep/**/b", []string{"deep/b", "deep/q/b"}, 0},
		{filepath.ToSlash(realRoot) + "/crates/a", []string{"crates/a"}, 0},
		{"crates/**z", nil, 1},
		{"crates/[", nil, 1},
	} {
		t.Run(tt.entry, func(t *testing.T) {
			writeFile(t, filepath.Join(root, "Cargo.toml"), fmt.Sprintf("[workspace]\nmembers = [%q]\n", tt.entry))
			got, warnings := publishablePackages(filepath.Join(link, "Cargo.toml"))
			dirs := []string{}
			for _, pkg := range got {
				dirs = append(dirs, path.Dir(pkg.ManifestPath))
			}
			if len(tt.want) == 0 {
				tt.want = []string{}
			}
			if !reflect.DeepEqual(dirs, tt.want) || len(warnings) != tt.warnings {
				t.Errorf("got %v %q, want %v and %d warning(s)", dirs, warnings, tt.want, tt.warnings)
			}
		})
	}
}

// The action passes a relative manifest path ("Cargo.toml" under the
// default path_prefix), so every rule comparing real paths must work
// from one as well as from an absolute path.
func TestPublishableFromARelativeManifestPath(t *testing.T) {
	root := writeManifests(t, map[string]string{
		"ws/abs/Cargo.toml": "[package]\nname = \"abs\"\nversion = \"1.0.0\"\n",
		"shared/Cargo.toml": "[package]\nname = \"shared\"\nversion = \"1.0.0\"\nworkspace = \"../ws\"\n",
	})
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	ws := filepath.ToSlash(filepath.Join(realRoot, "ws"))
	writeFile(t, filepath.Join(root, "ws", "Cargo.toml"), fmt.Sprintf(
		"[workspace]\nmembers = [\"../shared\", %q]\n[package]\nname = \"top\"\nversion = \"1.0.0\"\n"+
			"[dependencies]\nabs = { path = %q }\n", ws+"/abs", ws+"/abs"))
	t.Setenv("GITHUB_WORKSPACE", root)
	t.Chdir(filepath.Join(root, "ws"))

	got, warnings := publishablePackages("Cargo.toml")
	paths := []string{}
	for _, pkg := range got {
		paths = append(paths, pkg.ManifestPath)
	}
	want := []string{"../shared/Cargo.toml", "Cargo.toml", "abs/Cargo.toml"}
	if !reflect.DeepEqual(paths, want) || len(warnings) != 0 {
		t.Errorf("got %v %q, want %v and no warnings", paths, warnings, want)
	}
}

// A members entry from the repository must not make the action list
// directories outside the checkout, however it is spelled.
func TestPublishableMemberGlobsStayInTheCheckout(t *testing.T) {
	root := writeManifests(t, map[string]string{
		"outside/secret/Cargo.toml":     "[package]\nname = \"secret\"\nversion = \"1.0.0\"\n",
		"checkout/ws/a/Cargo.toml":      "[package]\nname = \"a\"\nversion = \"1.0.0\"\n",
		"checkout/side/b/Cargo.toml":    "[package]\nname = \"b\"\nversion = \"1.0.0\"\nworkspace = \"../../ws\"\n",
		"checkout/ws/deep/c/Cargo.toml": "[package]\nname = \"c\"\nversion = \"1.0.0\"\n",
	})
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(realRoot, "checkout")
	writeFile(t, filepath.Join(checkout, "ws", "Cargo.toml"),
		"[workspace]\nmembers = [\"/**\", \"/*/*\", \"../../*\", \"../../**\", \"../*/*\", \"**\"]\n")
	t.Setenv("GITHUB_WORKSPACE", checkout)
	confineListing(t, checkout)

	got, _ := publishablePackages(filepath.Join(checkout, "ws", "Cargo.toml"))
	if want := []string{"b", "a", "c"}; !reflect.DeepEqual(names(got), want) {
		t.Errorf("got %v, want %v", names(got), want)
	}
}

// Under GitHub Actions a root whose real path lies outside the
// workspace, or a workspace that cannot be resolved, reports no
// packages with a warning, and the scan reads and lists nothing.
func TestPublishableRootOutsideTheWorkspace(t *testing.T) {
	root := writeManifests(t, map[string]string{
		"outside/ws/a/Cargo.toml": "[package]\nname = \"a\"\nversion = \"1.0.0\"\n",
		"checkout/x/Cargo.toml":   "[package]\nname = \"x\"\nversion = \"1.0.0\"\n",
	})
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(realRoot, "checkout")
	outside := filepath.Join(realRoot, "outside", "ws")
	writeFile(t, filepath.Join(outside, "Cargo.toml"),
		"[package]\nname = \"root\"\nversion = \"1.0.0\"\n\n[workspace]\nmembers = [\"*\"]\n")
	// A root manifest that cannot be parsed yields no warning when it is
	// read, so the skip warning proves the check comes first.
	broken := filepath.Join(realRoot, "outside", "broken")
	writeFile(t, filepath.Join(broken, "Cargo.toml"), "[package\n")
	if err := os.Symlink(outside, filepath.Join(checkout, "link")); err != nil {
		t.Fatal(err)
	}

	cases := []struct{ name, workspace, manifest string }{
		{"root outside", checkout, filepath.Join(outside, "Cargo.toml")},
		{"unparsable root outside", checkout, filepath.Join(broken, "Cargo.toml")},
		{"link to a root outside", checkout, filepath.Join(checkout, "link", "Cargo.toml")},
		{"unresolvable workspace", filepath.Join(realRoot, "missing"), filepath.Join(checkout, "x", "Cargo.toml")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_WORKSPACE", tc.workspace)
			confineListing(t, filepath.Join(realRoot, "missing"))
			got, warnings := publishablePackages(tc.manifest)
			if len(got) != 0 || len(warnings) != 1 || !strings.Contains(warnings[0], "outside the workspace") {
				t.Errorf("got %v %q, want no packages and one warning", names(got), warnings)
			}
		})
	}

	// The same root inside the workspace, and with no workspace set.
	for _, workspace := range []string{realRoot, ""} {
		t.Setenv("GITHUB_WORKSPACE", workspace)
		got, warnings := publishablePackages(filepath.Join(outside, "Cargo.toml"))
		if want := []string{"root", "a"}; !reflect.DeepEqual(names(got), want) || len(warnings) != 0 {
			t.Errorf("GITHUB_WORKSPACE=%q: got %v %q, want %v", workspace, names(got), warnings, want)
		}
	}
}

// confineListing fails the test, without reading, the moment the walk
// lists a directory outside limit.
func confineListing(t *testing.T, limit string) {
	t.Helper()
	t.Cleanup(func() { listDir = os.ReadDir })
	listDir = func(dir string) ([]os.DirEntry, error) {
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil || !isWithin(limit, resolved) {
			t.Errorf("listed %s, outside %s", dir, limit)
			return nil, os.ErrPermission
		}
		return os.ReadDir(dir)
	}
}

// cargo 1.99: an absolute members entry naming a package overrides an
// exclude entry above it, like the relative form, and an absolute
// exclude entry excludes.
func TestPublishableAbsoluteMembersAndExcludes(t *testing.T) {
	root := writeManifests(t, map[string]string{
		"tools/kept/Cargo.toml": "[package]\nname = \"kept\"\nversion = \"1.0.0\"\n",
		"tools/gone/Cargo.toml": "[package]\nname = \"gone\"\nversion = \"1.0.0\"\n",
		"other/x/Cargo.toml":    "[package]\nname = \"x\"\nversion = \"1.0.0\"\n",
	})
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.ToSlash(realRoot)
	writeFile(t, filepath.Join(root, "Cargo.toml"), fmt.Sprintf(
		"[workspace]\nmembers = [%q, \"other/*\"]\nexclude = [\"tools\", %q]\n"+
			"[package]\nname = \"top\"\nversion = \"1.0.0\"\n[dependencies]\ngone = { path = \"tools/gone\" }\nx = { path = \"other/x\" }\n",
		abs+"/tools/kept", abs+"/other"))

	got, warnings := publishablePackages(filepath.Join(root, "Cargo.toml"))
	if want := []string{"top", "kept"}; !reflect.DeepEqual(names(got), want) || len(warnings) != 0 {
		t.Errorf("got %v %q, want %v and no warnings", names(got), warnings, want)
	}
}

// A path that aliases a package already visited, through a symlink
// back up the tree, must not visit it again: each pass would add a
// duplicate entry and follow the dependency one level deeper.
func TestPublishableSymlinkAliasCycle(t *testing.T) {
	root := writeManifests(t, map[string]string{
		"Cargo.toml":   "[workspace]\nmembers = [\"a\"]\n[package]\nname = \"top\"\nversion = \"1.0.0\"\n[dependencies]\nme = { path = \"link\" }\n",
		"a/Cargo.toml": "[package]\nname = \"a\"\nversion = \"1.0.0\"\n[dependencies]\nsibling = { path = \"../a-link\" }\n",
	})
	if err := os.Symlink(".", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a", filepath.Join(root, "a-link")); err != nil {
		t.Fatal(err)
	}
	got, warnings := publishablePackages(filepath.Join(root, "Cargo.toml"))
	paths := []string{}
	for _, pkg := range got {
		paths = append(paths, pkg.ManifestPath)
	}
	if want := []string{"Cargo.toml", "a/Cargo.toml"}; !reflect.DeepEqual(paths, want) || len(warnings) != 0 {
		t.Errorf("got %v %q, want %v and no warnings", paths, warnings, want)
	}
}

// Duplicate or equivalent members entries must not rescan the checkout
// once each: a short manifest could otherwise multiply the I/O of the
// walk. Every directory is listed at most once per workspace.
func TestPublishableMembersListEachDirectoryOnce(t *testing.T) {
	entries := strings.Repeat(`"**", "./**", "**/", `, 50)
	root := writeManifests(t, map[string]string{
		"Cargo.toml":       "[workspace]\nmembers = [" + entries + "\"a\"]\n",
		"a/Cargo.toml":     "[package]\nname = \"a\"\nversion = \"1.0.0\"\n",
		"b/c/Cargo.toml":   "[package]\nname = \"c\"\nversion = \"1.0.0\"\n",
		"b/c/d/Cargo.toml": "[package]\nname = \"d\"\nversion = \"1.0.0\"\n",
	})

	listed := map[string]int{}
	t.Cleanup(func() { listDir = os.ReadDir })
	listDir = func(dir string) ([]os.DirEntry, error) {
		listed[dir]++
		return os.ReadDir(dir)
	}

	got, _ := publishablePackages(filepath.Join(root, "Cargo.toml"))
	if want := []string{"a", "c", "d"}; !reflect.DeepEqual(names(got), want) {
		t.Errorf("got %v, want %v", names(got), want)
	}
	for dir, count := range listed {
		if count > 1 {
			t.Errorf("listed %s %d times", dir, count)
		}
	}
}
