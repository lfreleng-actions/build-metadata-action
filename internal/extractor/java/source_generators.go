// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"strconv"
	"strings"
)

// defaultPluginGroupID is the groupId Maven assumes when a plugin omits it.
const defaultPluginGroupID = "org.apache.maven.plugins"

// buildDirectory is the ${project.build.directory} reference that plugin
// documentation writes most default output locations against.
var buildDirectory = mavenPropertyOpen + "project.build.directory" + mavenPropertyClose

// pluginParam names a plugin parameter: its element within <configuration>,
// as a slash-separated path when nested, and the user property that sets it
// when the configuration does not.
type pluginParam struct {
	element  string
	property string
}

// generatorGoal describes where one goal of a generator writes sources.
type generatorGoal struct {
	output pluginParam
	// defaultDir is the plugin's documented default output.
	defaultDir string
	// defaultSuffix names a parameter whose value, when the goal has one,
	// is appended to defaultDir: protobuf's custom-plugin goals write to
	// <base>/<pluginId>.
	defaultSuffix *pluginParam
	// skip names the parameter that makes the goal return without
	// generating, when the plugin has one. antlr4's has none.
	skip *pluginParam
}

// sourceGenerator describes a Maven plugin that generates Java sources,
// with each fact taken from the plugin's own Mojo source or documentation.
type sourceGenerator struct {
	groupID    string
	artifactID string
	// subgroups extends groupID to its sub-groups, for a plugin published
	// under several: Swagger Codegen 3 as io.swagger.codegen.v3, and jOOQ's
	// commercial editions as org.jooq.pro and the like. Any other plugin
	// matches its groupId exactly, so a fork is not taken for it.
	subgroups bool
	goals     map[string]generatorGoal
	// sourceFolder names a folder beneath the output, for plugins that
	// register it as the source root rather than the output itself.
	sourceFolder *pluginParam
	// testPhase is the default output of an execution bound to
	// generate-test-sources, for plugins that pick it by phase.
	testPhase *phaseDefault
	// opaqueWhen names parameters that move the output somewhere this
	// action cannot see, such as an external configuration file. When one
	// is set, the generator is not reported.
	opaqueWhen []pluginParam
}

// phaseDefault is a default output a plugin picks by lifecycle phase,
// from the plugin version that introduced it. Before that version the
// goal's ordinary default applies.
type phaseDefault struct {
	dir   string
	since string
}

// Skip parameters shared by every goal of a plugin.
var (
	codegenSkip  = &pluginParam{"skip", "codegen.skip"}
	protocSkip   = &pluginParam{"skip", "protoc.skip"}
	protobufSkip = &pluginParam{"skip", "protobuf.skip"}
)

// sourceGenerators lists the generator plugins this action recognises. A
// plugin absent from the list is not reported, so the output can only
// under-report, never invent a directory.
var sourceGenerators = []sourceGenerator{
	{
		groupID:    "org.openapitools",
		artifactID: "openapi-generator-maven-plugin",
		goals: map[string]generatorGoal{
			"generate": {
				output:     pluginParam{"output", "openapi.generator.maven.plugin.output"},
				defaultDir: buildDirectory + "/generated-sources/openapi",
				skip:       codegenSkip,
			},
		},
		sourceFolder: &pluginParam{element: "configOptions/sourceFolder"},
		testPhase: &phaseDefault{
			dir:   buildDirectory + "/generated-test-sources/openapi",
			since: "6.1",
		},
	},
	{
		groupID:    "io.swagger",
		artifactID: "swagger-codegen-maven-plugin",
		subgroups:  true,
		goals: map[string]generatorGoal{
			"generate": {
				output:     pluginParam{"output", "swagger.codegen.maven.plugin.output"},
				defaultDir: buildDirectory + "/generated-sources/swagger",
				skip:       codegenSkip,
			},
		},
		sourceFolder: &pluginParam{element: "configOptions/sourceFolder"},
	},
	{
		groupID:    "org.xolstice.maven.plugins",
		artifactID: "protobuf-maven-plugin",
		goals: map[string]generatorGoal{
			"compile": {
				output:     pluginParam{"outputDirectory", "javaOutputDirectory"},
				defaultDir: buildDirectory + "/generated-sources/protobuf/java",
				skip:       protocSkip,
			},
			"test-compile": {
				output:     pluginParam{"outputDirectory", "javaTestOutputDirectory"},
				defaultDir: buildDirectory + "/generated-test-sources/protobuf/java",
				skip:       protocSkip,
			},
			"compile-custom": {
				output:        pluginParam{"outputDirectory", "protocPluginOutputDirectory"},
				defaultDir:    buildDirectory + "/generated-sources/protobuf",
				defaultSuffix: &pluginParam{"pluginId", "protocPluginId"},
				skip:          protocSkip,
			},
			"test-compile-custom": {
				output:        pluginParam{"outputDirectory", "protocPluginOutputDirectory"},
				defaultDir:    buildDirectory + "/generated-test-sources/protobuf",
				defaultSuffix: &pluginParam{"pluginId", "protocPluginId"},
				skip:          protocSkip,
			},
		},
	},
	{
		groupID:    "io.github.ascopes",
		artifactID: "protobuf-maven-plugin",
		goals: map[string]generatorGoal{
			"generate": {
				output:     pluginParam{element: "outputDirectory"},
				defaultDir: buildDirectory + "/generated-sources/protobuf",
				skip:       protobufSkip,
			},
			"generate-test": {
				output:     pluginParam{element: "outputDirectory"},
				defaultDir: buildDirectory + "/generated-test-sources/protobuf",
				skip:       protobufSkip,
			},
		},
	},
	{
		groupID:    "org.codehaus.mojo",
		artifactID: "jaxb2-maven-plugin",
		goals: map[string]generatorGoal{
			"xjc": {
				output:     pluginParam{element: "outputDirectory"},
				defaultDir: buildDirectory + "/generated-sources/jaxb",
				skip:       &pluginParam{"skipXjc", "xjc.skip"},
			},
			"testXjc": {
				output:     pluginParam{element: "outputDirectory"},
				defaultDir: buildDirectory + "/generated-test-sources/jaxb",
				skip:       &pluginParam{"skipTestXjc", "xjc.test.skip"},
			},
		},
	},
	{
		groupID:    "org.antlr",
		artifactID: "antlr4-maven-plugin",
		goals: map[string]generatorGoal{
			"antlr4": {
				output:     pluginParam{element: "outputDirectory"},
				defaultDir: buildDirectory + "/generated-sources/antlr4",
			},
		},
	},
	{
		// jOOQ resolves its default against the module directory rather
		// than the build directory, and reads its target from an external
		// file or a different base directory when told to.
		groupID:    "org.jooq",
		artifactID: "jooq-codegen-maven",
		subgroups:  true,
		goals: map[string]generatorGoal{
			"generate": {
				output:     pluginParam{element: "generator/target/directory"},
				defaultDir: "target/generated-sources/jooq",
				skip:       &pluginParam{"skip", "jooq.codegen.skip"},
			},
		},
		opaqueWhen: []pluginParam{
			{"configurationFile", "jooq.codegen.configurationFile"},
			{"configurationFiles/configurationFile", "jooq.codegen.configurationFiles"},
			{"basedir", "jooq.codegen.basedir"},
		},
	},
	{
		groupID:    "org.jsonschema2pojo",
		artifactID: "jsonschema2pojo-maven-plugin",
		goals: map[string]generatorGoal{
			"generate": {
				output:     pluginParam{"outputDirectory", "jsonschema2pojo.outputDirectory"},
				defaultDir: buildDirectory + "/generated-sources/jsonschema2pojo",
				skip:       &pluginParam{"skip", "jsonschema2pojo.skip"},
			},
		},
	},
}

// generatorFor returns the recognised generator a plugin declaration names,
// or nil.
func generatorFor(plugin Plugin) *sourceGenerator {
	groupID := pluginGroupID(plugin)
	artifactID := strings.TrimSpace(plugin.ArtifactID)
	for i := range sourceGenerators {
		generator := &sourceGenerators[i]
		if generator.artifactID != artifactID {
			continue
		}
		if groupID == generator.groupID || generator.subgroups && strings.HasPrefix(groupID, generator.groupID+".") {
			return generator
		}
	}
	return nil
}

// pluginGroupID returns a plugin's groupId, applying Maven's default.
func pluginGroupID(plugin Plugin) string {
	if groupID := strings.TrimSpace(plugin.GroupID); groupID != "" {
		return groupID
	}
	return defaultPluginGroupID
}

// pluginKey identifies a plugin across declarations as groupId:artifactId.
func pluginKey(plugin Plugin) string {
	return pluginGroupID(plugin) + ":" + strings.TrimSpace(plugin.ArtifactID)
}

// versionAtLeast compares the major.minor prefix of a plugin version with
// minimum, reporting false for ok when the version does not start with
// one, as an unresolved property or a range would not.
func versionAtLeast(version, minimum string) (meets, ok bool) {
	have, ok := majorMinor(version)
	if !ok {
		return false, false
	}
	want, ok := majorMinor(minimum)
	if !ok {
		return false, false
	}
	if have[0] != want[0] {
		return have[0] > want[0], true
	}
	return have[1] >= want[1], true
}

// majorMinor parses the leading major and minor numbers of a version such
// as 7.12.0 or 6.1-beta. A missing minor reads as 0.
func majorMinor(version string) ([2]int, bool) {
	var parts [2]int
	fields := strings.SplitN(strings.TrimSpace(version), ".", 3)
	for i := 0; i < len(parts) && i < len(fields); i++ {
		digits := fields[i]
		if end := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); end >= 0 {
			digits = digits[:end]
		}
		number, err := strconv.Atoi(digits)
		if err != nil {
			return parts, false
		}
		parts[i] = number
	}
	return parts, true
}
