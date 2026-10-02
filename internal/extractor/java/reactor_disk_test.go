// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// extractOutputs extracts a tree and returns generated_sources and
// generated_source_dirs, each nil when absent, for reactors where the flat
// list may rightly leave out a path generated_sources keeps.
func extractOutputs(t *testing.T, root string) ([]generatedSource, []string) {
	t.Helper()

	metadata, err := NewMavenExtractor().Extract(root)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	sources, _ := metadata.LanguageSpecific["generated_sources"].([]generatedSource)
	dirs, _ := metadata.LanguageSpecific["generated_source_dirs"].([]string)
	return sources, dirs
}

// linkTree creates directories, then relative links, under root, skipping
// the test where links cannot be made.
func linkTree(t *testing.T, root string, dirs []string, links map[string]string) {
	t.Helper()

	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0755); err != nil {
			t.Fatalf("failed to create %s: %v", dir, err)
		}
	}
	for link, target := range links {
		if err := os.Symlink(filepath.FromSlash(target), filepath.Join(root, filepath.FromSlash(link))); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
	}
}

// A link can carry a generator into hand-written sources under another
// name, and past one even the build directory may hold anything, so a
// path reached through a link is withheld. A source root a link moves
// counts where it resolves to, so a generator writing there is refused
// even though the path it names is only build output.
func TestGeneratedSourcesFollowLinks(t *testing.T) {
	beside := "${project.basedir}/generated"
	output := "${project.build.directory}/generated-sources/xjc"
	tests := []struct {
		name   string
		output string
		dirs   []string
		links  map[string]string
		want   string
	}{
		{name: "beside the sources, no link", output: beside, want: "generated"},
		{name: "through a link into the sources", output: beside, dirs: []string{"src/main/java"},
			links: map[string]string{"generated": "src/main/java"}},
		{name: "through a link elsewhere", output: beside, dirs: []string{"elsewhere"},
			links: map[string]string{"generated": "elsewhere"}},
		{name: "build output, no link", output: output, want: "target/generated-sources/xjc"},
		{name: "build output through a linked build directory", output: output, dirs: []string{"build"},
			links: map[string]string{"target": "build"}},
		{name: "build output a source root links to", output: output,
			dirs:  []string{"src/main", "target/generated-sources/xjc"},
			links: map[string]string{"src/main/java": "../../target/generated-sources/xjc"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{"pom.xml": projectPOM(withPlugins(xjcInto(tc.output)))})
			linkTree(t, root, tc.dirs, tc.links)

			var want []generatedSource
			var wantDirs []string
			if tc.want != "" {
				want = []generatedSource{rootSource(tc.want, "jaxb2-maven-plugin", derivationConfigured)}
				wantDirs = []string{tc.want}
			}
			sources, dirs := extractOutputs(t, root)
			assertSources(t, sources, want)
			if !reflect.DeepEqual(dirs, wantDirs) {
				t.Errorf("generated_source_dirs = %v, want %v", dirs, wantDirs)
			}
		})
	}
}

// A generator may write into a nested module's directory, where that
// module's source roots decide what is safe: its declared ones, or, when
// it inherits from a parent off disk, anything outside its build output.
func TestGeneratedSourcesIntoNestedModule(t *testing.T) {
	ownRoots := parentPOM("app", "1.0.0", "")
	unreadParent := childPOM("app", "remote-parent", "1.0.0", "", "")
	tests := []struct {
		name     string
		app      string
		output   string
		want     string
		wantDirs []string
	}{
		{name: "into its sources", app: ownRoots, output: "app/src/main/java/gen"},
		{name: "beside its sources", app: ownRoots, output: "app/gen", want: "app/gen", wantDirs: []string{"app/gen"}},
		{name: "unread parent, outside its build output", app: unreadParent, output: "app/gen"},
		// The flat list, applied in every module, would name app/target/gen
		// inside app as well, which is not its build output.
		{name: "unread parent, inside its build output", app: unreadParent, output: "app/target/gen", want: "app/target/gen"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules><module>app</module></modules>` + withPlugins(xjcInto("${project.basedir}/"+tc.output))),
				"app/pom.xml": tc.app,
			})

			var want []generatedSource
			if tc.want != "" {
				want = []generatedSource{rootSource(tc.want, "jaxb2-maven-plugin", derivationConfigured)}
			}
			sources, dirs := extractOutputs(t, root)
			assertSources(t, sources, want)
			if !reflect.DeepEqual(dirs, tc.wantDirs) {
				t.Errorf("generated_source_dirs = %v, want %v", dirs, tc.wantDirs)
			}
		})
	}
}

// A reactor root inheriting from a parent off disk, as most do, has source
// roots the action cannot see, and the parent may place one in a module
// below it, so a path there is reported only inside a build directory,
// which mvn clean empties: gen's own, here. The flat list then also
// needs the root's own build directory to hold it, as it does for
// target, since the list is applied in the root too.
func TestGeneratedSourcesBelowRootWithUnreadParent(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "beside the module's sources", output: "${project.basedir}/generated"},
		{name: "in the module's build output", output: "${project.build.directory}/xjc", want: "target/xjc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml": childPOM("root", "remote-parent", "1.0.0", "", `
    <packaging>pom</packaging>
    <modules><module>gen</module></modules>`),
				"gen/pom.xml": projectPOM(withPlugins(xjcInto(tc.output))),
			})

			var want []generatedSource
			var wantDirs []string
			if tc.want != "" {
				want = []generatedSource{{Module: "gen", Path: tc.want, Plugin: "jaxb2-maven-plugin", Derivation: derivationConfigured}}
				wantDirs = []string{tc.want}
			}
			sources, dirs := extractOutputs(t, root)
			assertSources(t, sources, want)
			if !reflect.DeepEqual(dirs, wantDirs) {
				t.Errorf("generated_source_dirs = %v, want %v", dirs, wantDirs)
			}
		})
	}
}

// The flat list applies each path in every module, so a path that reaches
// hand-written sources through a link in any module stays out of it.
// generated_sources keeps the entry, which names the module it is safe in.
func TestGeneratedSourceDirsFollowLinksInEveryModule(t *testing.T) {
	tests := []struct {
		name     string
		links    map[string]string
		wantDirs []string
	}{
		{name: "no link", wantDirs: []string{"generated"}},
		{name: "a link in another module", links: map[string]string{"app/generated": "src/main/java"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules>
        <module>gen</module>
        <module>app</module>
    </modules>`),
				"gen/pom.xml": projectPOM(withPlugins(xjcInto("${project.basedir}/generated"))),
				"app/pom.xml": projectPOM(""),
			})
			linkTree(t, root, []string{"app/src/main/java"}, tc.links)

			sources, dirs := extractOutputs(t, root)
			assertSources(t, sources, []generatedSource{
				{Module: "gen", Path: "generated", Plugin: "jaxb2-maven-plugin", Derivation: derivationConfigured},
			})
			if !reflect.DeepEqual(dirs, tc.wantDirs) {
				t.Errorf("generated_source_dirs = %v, want %v", dirs, tc.wantDirs)
			}
		})
	}
}
