// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"reflect"
	"testing"
)

// xjcInto returns a jaxb2-maven-plugin declaration whose xjc goal
// generates into output.
func xjcInto(output string) string {
	return `
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>xjc</goal></goals>
                            <configuration><outputDirectory>` + output + `</outputDirectory></configuration>
                        </execution>
                    </executions>
                </plugin>`
}

// buildHelperAdding returns a build-helper-maven-plugin declaration adding
// one source root.
func buildHelperAdding(source string) string {
	return `
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>build-helper-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>add-source</goal></goals>
                            <configuration><sources><source>` + source + `</source></sources></configuration>
                        </execution>
                    </executions>
                </plugin>`
}

// jdkProfile returns a profile the action cannot settle, activated by the
// JDK, setting properties and declaring build plugins.
func jdkProfile(id, properties, plugins string) string {
	if plugins != "" {
		plugins = "<build><plugins>" + plugins + "</plugins></build>"
	}
	return `
        <profile>
            <id>` + id + `</id>
            <activation><jdk>[17,)</jdk></activation>
            <properties>` + properties + `</properties>` + plugins + `
        </profile>`
}

// Maven can activate several profiles together, so a source root resting
// on properties two profiles set may take a form no single profile's
// reading shows: here generated/sources, the generator's output. Such a
// root counts as unresolved, leaving the module only its build output. A
// root a profile declares takes effect with that profile active, so one
// other profile setting its properties is already a combination. A single
// profile is read exactly, and the body's build-helper roots under every
// profile's properties.
func TestGeneratedSourcesCombineProfileProperties(t *testing.T) {
	generator := xjcInto("${project.basedir}/generated/sources")
	declaredRoot := func(profiles string) string {
		return projectPOM(`
    <properties><root.dir>src</root.dir><lang.dir>main/java</lang.dir></properties>
    <build>
        <sourceDirectory>${root.dir}/${lang.dir}</sourceDirectory>
        <plugins>` + generator + `</plugins>
    </build>
    <profiles>` + profiles + `
    </profiles>`)
	}
	addedRoot := func(plugins, profiles string) string {
		return projectPOM(`
    <properties><root.dir>src</root.dir><lang.dir>sources</lang.dir></properties>` + withPlugins(plugins+generator) + `
    <profiles>` + profiles + `
    </profiles>`)
	}
	moveRoot := jdkProfile("root", "<root.dir>generated</root.dir>", "")
	moveLang := jdkProfile("lang", "<lang.dir>sources</lang.dir>", "")
	helper := buildHelperAdding("${root.dir}/${lang.dir}")
	reported := []generatedSource{rootSource("generated/sources", "jaxb2-maven-plugin", derivationConfigured)}

	tests := []struct {
		name string
		pom  string
		want []generatedSource
	}{
		{name: "declared root two profiles set together", pom: declaredRoot(moveRoot + moveLang)},
		{name: "declared root one profile sets", pom: declaredRoot(moveRoot), want: reported},
		{name: "root a profile adds, another profile beside it", pom: addedRoot("", jdkProfile("helper", "", helper)+moveRoot)},
		{name: "root a profile adds, alone", pom: addedRoot("", jdkProfile("helper", "", helper)), want: reported},
		{name: "root the body adds, one profile moving it", pom: addedRoot(helper, moveRoot)},
		{name: "root the body adds, no profile", pom: addedRoot(helper, ""), want: reported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertSources(t, generatedSourcesIn(t, extractPOM(t, tc.pom)), tc.want)
		})
	}
}

// A profile can move the build directory as well as set properties, so a
// source root written against ${project.build.directory} is read with
// each profile's directory too, and a profile moving it counts among those
// setting the root's properties. Here the root lands on out/manual, the
// generator's output, only under the profile moving the directory.
func TestGeneratedSourcesReadProfileBuildDirectory(t *testing.T) {
	moveBuild := `
        <profile>
            <id>out</id>
            <activation><jdk>[17,)</jdk></activation>
            <build><directory>${project.basedir}/out</directory></build>
        </profile>`
	setSub := jdkProfile("sub", "<sub.dir>manual</sub.dir>", "")
	pom := func(sub, profiles string) string {
		return projectPOM(`
    <properties><sub.dir>` + sub + `</sub.dir></properties>
    <build>
        <sourceDirectory>${project.build.directory}/${sub.dir}</sourceDirectory>
        <plugins>` + xjcInto("${project.basedir}/out/manual") + `</plugins>
    </build>
    <profiles>` + profiles + `
    </profiles>`)
	}
	reported := []generatedSource{rootSource("out/manual", "jaxb2-maven-plugin", derivationConfigured)}

	tests := []struct {
		name string
		pom  string
		want []generatedSource
	}{
		{name: "no profile", pom: pom("manual", ""), want: reported},
		{name: "a profile moving the build directory", pom: pom("manual", moveBuild)},
		{name: "with another profile setting the rest", pom: pom("other", moveBuild+setSub)},
		{name: "the other profile alone", pom: pom("other", setSub), want: reported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertSources(t, generatedSourcesIn(t, extractPOM(t, tc.pom)), tc.want)
		})
	}
}

// A reference nested in another, ${root.${flavour}}, names a property only
// substitution settles, so any profile setting anything may bear on it,
// and two such profiles leave the root unresolved. Together these two move
// it onto the generator's output; one alone does not.
func TestGeneratedSourcesNestedPropertyReference(t *testing.T) {
	pom := func(profiles string) string {
		return projectPOM(`
    <properties>
        <flavour>a</flavour>
        <root.a>src/main/java</root.a>
        <root.b>src/b</root.b>
    </properties>
    <build>
        <sourceDirectory>${root.${flavour}}</sourceDirectory>
        <plugins>` + xjcInto("${project.basedir}/generated") + `</plugins>
    </build>
    <profiles>` + profiles + `
    </profiles>`)
	}
	flavourB := jdkProfile("flavour", "<flavour>b</flavour>", "")
	moveB := jdkProfile("moved", "<root.b>generated</root.b>", "")

	assertSources(t, generatedSourcesIn(t, extractPOM(t, pom(flavourB+moveB))), nil)
	assertSources(t, generatedSourcesIn(t, extractPOM(t, pom(flavourB))), []generatedSource{
		rootSource("generated", "jaxb2-maven-plugin", derivationConfigured),
	})
}

// A module entry is read with each profile's properties too: under a
// profile the action does not see active it names another module, whose
// source roots the flat list must respect. An entry resting on properties
// two profiles could set together cannot be read with certainty, so the
// flat list is withheld. generated_sources keeps gen's entry throughout.
func TestGeneratedSourceDirsReadModulesUnderProfiles(t *testing.T) {
	tests := []struct {
		name     string
		profiles string
		wantDirs []string
	}{
		{name: "no profile", wantDirs: []string{"generated"}},
		{name: "a profile naming a module with that root", profiles: jdkProfile("alt", "<group.dir>alt</group.dir>", "")},
		{name: "a profile naming a plain module", profiles: jdkProfile("alt", "<group.dir>alt2</group.dir>", ""), wantDirs: []string{"generated"}},
		{
			name: "two profiles together",
			profiles: jdkProfile("group", "<group.dir>alt2</group.dir>", "") +
				jdkProfile("name", "<module.dir>other</module.dir>", ""),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <properties><group.dir>mods</group.dir><module.dir>lib</module.dir></properties>
    <modules>
        <module>gen</module>
        <module>${group.dir}/${module.dir}</module>
    </modules>
    <profiles>` + tc.profiles + `
    </profiles>`),
				"gen/pom.xml":        projectPOM(withPlugins(xjcInto("${project.basedir}/generated"))),
				"mods/lib/pom.xml":   projectPOM(""),
				"mods/other/pom.xml": projectPOM(""),
				"alt2/lib/pom.xml":   projectPOM(""),
				"alt/lib/pom.xml":    projectPOM("<build><sourceDirectory>generated</sourceDirectory></build>"),
			})

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

// A module a profile declares joins only with that profile active, so
// another profile setting a property its entry rests on is already a
// combination of two, one no single reading shows: here it names alt/lib,
// whose source root is gen's output. The entry counts as unread.
func TestGeneratedSourceDirsReadProfileModulesWithOtherProfiles(t *testing.T) {
	declaring := `
        <profile>
            <id>extra</id>
            <activation><jdk>[17,)</jdk></activation>
            <modules><module>${group.dir}/lib</module></modules>
        </profile>`
	tests := []struct {
		name     string
		other    string
		wantDirs []string
	}{
		{name: "declaring profile alone", wantDirs: []string{"generated"}},
		{name: "another profile setting its property", other: jdkProfile("moved", "<group.dir>alt</group.dir>", "")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <properties><group.dir>mods</group.dir></properties>
    <modules><module>gen</module></modules>
    <profiles>` + declaring + tc.other + `
    </profiles>`),
				"gen/pom.xml":      projectPOM(withPlugins(xjcInto("${project.basedir}/generated"))),
				"mods/lib/pom.xml": projectPOM(""),
				"alt/lib/pom.xml":  projectPOM("<build><sourceDirectory>generated</sourceDirectory></build>"),
			})

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
