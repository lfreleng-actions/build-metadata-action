// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"
)

const (
	openAPI        = "openapi-generator-maven-plugin"
	compilerPlugin = "maven-compiler-plugin"
)

// projectPOM wraps elements in a minimal jar POM.
func projectPOM(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>generated</artifactId>
    <version>1.0.0</version>
` + body + `
</project>`
}

// withPlugins wraps plugin declarations in <build><plugins>.
func withPlugins(plugins string) string {
	return "<build><plugins>" + plugins + "</plugins></build>"
}

// antlrBound is an antlr4 plugin with its goal bound, the smallest
// declaration that runs a generator.
const antlrBound = `
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>antlr4</goal></goals>
                        </execution>
                    </executions>
                </plugin>`

// antlrDefault is what antlrBound generates in the scanned module.
var antlrDefault = rootSource("target/generated-sources/antlr4", "antlr4-maven-plugin", derivationPluginDefault)

// openAPIExecution is an openapi-generator plugin with one generate
// execution carrying the given configuration, and optionally a version.
func openAPIExecution(version, phase, configuration string) string {
	if version != "" {
		version = "<version>" + version + "</version>"
	}
	if phase != "" {
		phase = "<phase>" + phase + "</phase>"
	}
	return `
                <plugin>
                    <groupId>org.openapitools</groupId>
                    <artifactId>openapi-generator-maven-plugin</artifactId>
                    ` + version + `
                    <executions>
                        <execution>
                            ` + phase + `
                            <goals><goal>generate</goal></goals>
                            <configuration>` + configuration + `</configuration>
                        </execution>
                    </executions>
                </plugin>`
}

// annotationOutputs are the compiler's main and test outputs for a
// processor that reaches both compilations by default.
var annotationOutputs = []generatedSource{
	rootSource("target/generated-sources/annotations", compilerPlugin, derivationPluginDefault),
	rootSource("target/generated-test-sources/test-annotations", compilerPlugin, derivationPluginDefault),
}

// rootSource is a generated source recorded against the scanned POM.
func rootSource(dir, plugin, derivation string) generatedSource {
	return generatedSource{Module: ".", Path: dir, Plugin: plugin, Derivation: derivation}
}

// generatedSourcesIn returns what extraction recorded for a project, nil
// when it recorded nothing, checking the flat directory list agrees.
func generatedSourcesIn(t *testing.T, ls map[string]interface{}) []generatedSource {
	t.Helper()

	raw, present := ls["generated_sources"]
	if !present {
		if dirs, ok := ls["generated_source_dirs"]; ok {
			t.Fatalf("generated_source_dirs = %v without generated_sources", dirs)
		}
		return nil
	}
	sources, ok := raw.([]generatedSource)
	if !ok {
		t.Fatalf("generated_sources is %T, want []generatedSource", raw)
	}
	var wantDirs []string
	seen := map[string]bool{}
	for _, source := range sources {
		if !seen[source.Path] {
			seen[source.Path] = true
			wantDirs = append(wantDirs, source.Path)
		}
	}
	assertStringSlice(t, ls, "generated_source_dirs", wantDirs)
	return sources
}

// writeTree writes files, keyed by slash-separated relative path, under a
// fresh directory and returns it with symlinks resolved, so the result
// can also serve as GITHUB_WORKSPACE on a platform whose temp directory
// sits behind a link.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("failed to resolve temp dir: %v", err)
	}
	for name, content := range files {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatalf("failed to create %s: %v", filepath.Dir(file), err)
		}
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", file, err)
		}
	}
	return root
}

// extractTree extracts the project at root and returns its generated sources.
func extractTree(t *testing.T, root string) []generatedSource {
	t.Helper()

	metadata, err := NewMavenExtractor().Extract(root)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	return generatedSourcesIn(t, metadata.LanguageSpecific)
}

func assertSources(t *testing.T, got, want []generatedSource) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("generated sources:\n got  %+v\n want %+v", got, want)
	}
}

// Each case is a single-module POM. The shapes marked ONAP are taken
// from onap/cps and onap/cps-ncmp-dmi-plugin, the reactors whose scan
// prompted these outputs.
func TestGeneratedSourcesFromPlugins(t *testing.T) {
	tests := []struct {
		name string
		pom  string
		want []generatedSource
	}{
		{
			name: "no generator records nothing",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.apache.maven.plugins</groupId>
                    <artifactId>maven-surefire-plugin</artifactId>
                </plugin>`)),
		},
		{
			name: "openapi default output and source folder",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.openapitools</groupId>
                    <artifactId>openapi-generator-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>generate</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/generated-sources/openapi", openAPI, derivationPluginDefault),
			},
		},
		{
			// ONAP dmi-service: one execution per API, configured in the
			// execution rather than at plugin level.
			name: "openapi source folder set per execution",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.openapitools</groupId>
                    <artifactId>openapi-generator-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>dmi-code-gen</id>
                            <goals><goal>generate</goal></goals>
                            <configuration>
                                <generatorName>spring</generatorName>
                                <configOptions>
                                    <sourceFolder>src/gen/java</sourceFolder>
                                </configOptions>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/generated-sources/openapi/src/gen/java", openAPI, derivationPluginDefault),
			},
		},
		{
			// Without a configured sourceFolder each generator picks its own
			// beneath the output, so the configured output itself is
			// reported, holding it whatever the generator.
			name: "openapi configured output",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.openapitools</groupId>
                    <artifactId>openapi-generator-maven-plugin</artifactId>
                    <configuration>
                        <output>${project.build.directory}/generated-sources/api</output>
                    </configuration>
                    <executions>
                        <execution>
                            <goals><goal>generate</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/generated-sources/api", openAPI, derivationConfigured),
			},
		},
		{
			name: "openapi output and source folder both configured",
			pom: projectPOM(withPlugins(openAPIExecution("", "", `
                                <output>${project.build.directory}/api</output>
                                <configOptions>
                                    <sourceFolder>java</sourceFolder>
                                </configOptions>`))),
			want: []generatedSource{
				rootSource("target/api/java", openAPI, derivationConfigured),
			},
		},
		{
			// JAX-RS generators default their sourceFolder to src/gen/java
			// while others use src/main/java; the whole output holds either.
			name: "openapi JAX-RS generator without a source folder",
			pom:  projectPOM(withPlugins(openAPIExecution("", "", "<generatorName>jaxrs-cxf</generatorName>"))),
			want: []generatedSource{
				rootSource("target/generated-sources/openapi", openAPI, derivationPluginDefault),
			},
		},
		{
			// Outside the build directory the output may share a directory
			// with hand-written files, so without a source folder naming
			// where Java lands nothing is safe to report.
			name: "openapi output into resources without a source folder",
			pom: projectPOM(withPlugins(openAPIExecution("", "", `
                                <generatorName>openapi-yaml</generatorName>
                                <output>${project.basedir}/src/main/resources/static</output>`))),
		},
		{
			// Maven feeds a POM property to the parameter whose user
			// property it names.
			name: "openapi output set through its user property",
			pom: projectPOM(`
    <properties>
        <openapi.generator.maven.plugin.output>${project.basedir}/generated</openapi.generator.maven.plugin.output>
    </properties>` + withPlugins(openAPIExecution("", "", `
                                <configOptions>
                                    <sourceFolder>src/gen/java</sourceFolder>
                                </configOptions>`))),
			want: []generatedSource{
				rootSource("generated/src/gen/java", openAPI, derivationConfigured),
			},
		},
		{
			// Generating into the module itself writes to src/main/java;
			// reporting that would exclude the hand-written code too.
			name: "openapi output into the module is refused",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.openapitools</groupId>
                    <artifactId>openapi-generator-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>generate</goal></goals>
                            <configuration>
                                <output>${project.basedir}</output>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)),
		},
		{
			// openapi-generator picks the test default by phase from 6.1.0;
			// the version here arrives through a property.
			name: "openapi bound to generate-test-sources from 6.1",
			pom: projectPOM(`
    <properties>
        <openapi.version>7.12.0</openapi.version>
    </properties>` + withPlugins(openAPIExecution("${openapi.version}", "generate-test-sources", ""))),
			want: []generatedSource{
				rootSource("target/generated-test-sources/openapi", openAPI, derivationPluginDefault),
			},
		},
		{
			name: "openapi bound to generate-test-sources before 6.1",
			pom:  projectPOM(withPlugins(openAPIExecution("6.0.1", "generate-test-sources", ""))),
			want: []generatedSource{
				rootSource("target/generated-sources/openapi", openAPI, derivationPluginDefault),
			},
		},
		{
			// Without a version the default could be either; leave it out.
			name: "openapi bound to generate-test-sources at an unknown version",
			pom:  projectPOM(withPlugins(openAPIExecution("", "generate-test-sources", ""))),
		},
		{
			name: "swagger-codegen v3 matched through its group",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>io.swagger.codegen.v3</groupId>
                    <artifactId>swagger-codegen-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>generate</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/generated-sources/swagger",
					"swagger-codegen-maven-plugin", derivationPluginDefault),
			},
		},
		{
			// Only Swagger and jOOQ publish under sub-groups; anywhere else a
			// sub-group is another plugin, such as a fork, with its own defaults.
			name: "antlr4 artifactId under a sub-group of its group",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.antlr.fork</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>antlr4</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
		},
		{
			name: "protobuf compile and gRPC custom plugin",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.xolstice.maven.plugins</groupId>
                    <artifactId>protobuf-maven-plugin</artifactId>
                    <configuration>
                        <pluginId>grpc-java</pluginId>
                    </configuration>
                    <executions>
                        <execution>
                            <goals>
                                <goal>compile</goal>
                                <goal>compile-custom</goal>
                                <goal>compile-python</goal>
                            </goals>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/generated-sources/protobuf/java", "protobuf-maven-plugin", derivationPluginDefault),
				rootSource("target/generated-sources/protobuf/grpc-java", "protobuf-maven-plugin", derivationPluginDefault),
			},
		},
		{
			// The custom goal cannot run without a pluginId, so there is
			// no directory to name.
			name: "protobuf custom goal without a plugin id",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.xolstice.maven.plugins</groupId>
                    <artifactId>protobuf-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>compile-custom</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
		},
		{
			name: "protobuf from the ascopes plugin",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>io.github.ascopes</groupId>
                    <artifactId>protobuf-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>generate</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/generated-sources/protobuf", "protobuf-maven-plugin", derivationPluginDefault),
			},
		},
		{
			name: "jaxb2 main and test goals",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>main</id>
                            <goals><goal>xjc</goal></goals>
                        </execution>
                        <execution>
                            <id>tests</id>
                            <goals><goal>testXjc</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/generated-sources/jaxb", "jaxb2-maven-plugin", derivationPluginDefault),
				rootSource("target/generated-test-sources/jaxb", "jaxb2-maven-plugin", derivationPluginDefault),
			},
		},
		{
			// ONAP cps-path-parser.
			name: "antlr4 with no configuration",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>antlr4</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/generated-sources/antlr4", "antlr4-maven-plugin", derivationPluginDefault),
			},
		},
		{
			// A generator goal runs only when an execution binds it. ONAP
			// cps-events declares jsonschema2pojo like this and relies on a
			// parent for the execution; on its own it generates nothing.
			name: "generator with no bound execution",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.jsonschema2pojo</groupId>
                    <artifactId>jsonschema2pojo-maven-plugin</artifactId>
                    <configuration>
                        <sourceDirectory>${basedir}/src/main/resources/schemas</sourceDirectory>
                    </configuration>
                </plugin>`)),
		},
		{
			name: "execution that names no goal",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>unbound</id>
                        </execution>
                    </executions>
                </plugin>`)),
		},
		{
			name: "jooq configured target directory",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.jooq</groupId>
                    <artifactId>jooq-codegen-maven</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>generate</goal></goals>
                        </execution>
                    </executions>
                    <configuration>
                        <generator>
                            <target>
                                <packageName>com.example.db</packageName>
                                <directory>target/generated-sources/db</directory>
                            </target>
                        </generator>
                    </configuration>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/generated-sources/db", "jooq-codegen-maven", derivationConfigured),
			},
		},
		{
			// jOOQ's default is relative to the module, not the build
			// directory, so a moved build directory does not move it.
			name: "jooq default ignores the build directory",
			pom: projectPOM(`
    <build>
        <directory>build</directory>
        <plugins>
            <plugin>
                <groupId>org.jooq.pro</groupId>
                <artifactId>jooq-codegen-maven</artifactId>
                <executions>
                    <execution>
                        <goals><goal>generate</goal></goals>
                    </execution>
                </executions>
            </plugin>
        </plugins>
    </build>`),
			want: []generatedSource{
				rootSource("target/generated-sources/jooq", "jooq-codegen-maven", derivationPluginDefault),
			},
		},
		{
			name: "jooq configured from an external file",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.jooq</groupId>
                    <artifactId>jooq-codegen-maven</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>generate</goal></goals>
                        </execution>
                    </executions>
                    <configuration>
                        <configurationFile>src/main/resources/jooq.xml</configurationFile>
                    </configuration>
                </plugin>`)),
		},
		{
			// Generating into a package inside src/main/java would share it
			// with hand-written classes, so the path is refused too.
			name: "output inside a source directory is refused",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>xjc</goal></goals>
                            <configuration>
                                <outputDirectory>${project.basedir}/src/main/java/com/example/model</outputDirectory>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)),
		},
		{
			// A module disables an execution, often an inherited one, by
			// binding it to the phase none.
			name: "execution disabled with phase none",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <phase>none</phase>
                            <goals><goal>antlr4</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
		},
		{
			name: "skipped in configuration",
			pom:  projectPOM(withPlugins(openAPIExecution("", "", "<skip>true</skip>"))),
		},
		{
			name: "skipped through the user property",
			pom: projectPOM(`
    <properties>
        <skipCodegen>true</skipCodegen>
        <codegen.skip>${skipCodegen}</codegen.skip>
    </properties>` + withPlugins(openAPIExecution("", "", ""))),
		},
		{
			// jaxb2 skips each goal separately, so skipping xjc leaves the
			// test goal running.
			name: "skip that applies to one goal",
			pom: projectPOM(`
    <properties>
        <xjc.skip>true</xjc.skip>
    </properties>` + withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>xjc</goal><goal>testXjc</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/generated-test-sources/jaxb", "jaxb2-maven-plugin", derivationPluginDefault),
			},
		},
		{
			name: "skip set false still generates",
			pom:  projectPOM(withPlugins(openAPIExecution("", "", "<skip>false</skip>"))),
			want: []generatedSource{
				rootSource("target/generated-sources/openapi", openAPI, derivationPluginDefault),
			},
		},
		{
			name: "build directory override moves defaults",
			pom: projectPOM(`
    <build>
        <directory>${project.basedir}/out</directory>
        <plugins>` + antlrBound + `
        </plugins>
    </build>`),
			want: []generatedSource{
				rootSource("out/generated-sources/antlr4", "antlr4-maven-plugin", derivationPluginDefault),
			},
		},
		{
			// pluginManagement sets defaults for modules that opt in; on
			// its own the plugin never runs.
			name: "generator only under pluginManagement",
			pom: projectPOM(`
    <build>
        <pluginManagement>
            <plugins>
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                </plugin>
            </plugins>
        </pluginManagement>
    </build>`),
		},
		{
			// The managed declaration supplies both the execution and the
			// output to a plugin the module merely names.
			name: "pluginManagement supplies the execution and output",
			pom: projectPOM(`
    <build>
        <pluginManagement>
            <plugins>
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <configuration>
                        <outputDirectory>${project.build.directory}/grammar</outputDirectory>
                    </configuration>
                    <executions>
                        <execution>
                            <goals><goal>antlr4</goal></goals>
                        </execution>
                    </executions>
                </plugin>
            </plugins>
        </pluginManagement>
        <plugins>
            <plugin>
                <groupId>org.antlr</groupId>
                <artifactId>antlr4-maven-plugin</artifactId>
            </plugin>
        </plugins>
    </build>`),
			want: []generatedSource{
				rootSource("target/grammar", "antlr4-maven-plugin", derivationConfigured),
			},
		},
		{
			// ONAP provmns-api generates only under a manual profile; the
			// default build never runs it.
			name: "generator only in a profile",
			pom: projectPOM(`
    <profiles>
        <profile>
            <id>generate</id>
            <build>
                <plugins>
                    <plugin>
                        <groupId>org.antlr</groupId>
                        <artifactId>antlr4-maven-plugin</artifactId>
                    </plugin>
                </plugins>
            </build>
        </profile>
    </profiles>`),
		},
		{
			name: "unresolvable, absolute and escaping outputs are refused",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>unresolved</id>
                            <goals><goal>xjc</goal></goals>
                            <configuration>
                                <outputDirectory>${undefined.dir}/jaxb</outputDirectory>
                            </configuration>
                        </execution>
                        <execution>
                            <id>absolute</id>
                            <goals><goal>xjc</goal></goals>
                            <configuration>
                                <outputDirectory>/var/tmp/jaxb</outputDirectory>
                            </configuration>
                        </execution>
                        <execution>
                            <id>windows</id>
                            <goals><goal>xjc</goal></goals>
                            <configuration>
                                <outputDirectory>C:\build\jaxb</outputDirectory>
                            </configuration>
                        </execution>
                        <execution>
                            <id>escaping</id>
                            <goals><goal>xjc</goal></goals>
                            <configuration>
                                <outputDirectory>${project.basedir}/../shared/jaxb</outputDirectory>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertSources(t, generatedSourcesIn(t, extractPOM(t, tc.pom)), tc.want)
		})
	}
}

func TestGeneratedSourcesFromAnnotationProcessors(t *testing.T) {
	mapstructDependency := func(scope string) string {
		return `
    <dependencies>
        <dependency>
            <groupId>org.mapstruct</groupId>
            <artifactId>mapstruct-processor</artifactId>
            <scope>` + scope + `</scope>
        </dependency>
    </dependencies>`
	}
	querydslDependency := func(classifier string) string {
		if classifier != "" {
			classifier = "<classifier>" + classifier + "</classifier>"
		}
		return `
    <dependencies>
        <dependency>
            <groupId>com.querydsl</groupId>
            <artifactId>querydsl-apt</artifactId>
            ` + classifier + `
            <scope>provided</scope>
        </dependency>
    </dependencies>`
	}

	tests := []struct {
		name string
		pom  string
		want []generatedSource
	}{
		{
			// ONAP cps-rest: MapStruct as a plain dependency, discovered
			// on the classpath, with no compiler plugin declared. Maven's
			// test compilation sees the main classpath too.
			name: "processor on the classpath",
			pom:  projectPOM(mapstructDependency("provided")),
			want: annotationOutputs,
		},
		{
			// Plugin-level configuration applies to both compilations.
			name: "processor on annotationProcessorPaths",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessorPaths>
                            <path>
                                <groupId>org.projectlombok</groupId>
                                <artifactId>lombok</artifactId>
                            </path>
                            <path>
                                <groupId>org.mapstruct</groupId>
                                <artifactId>mapstruct-processor</artifactId>
                            </path>
                        </annotationProcessorPaths>
                    </configuration>
                </plugin>`)),
			want: annotationOutputs,
		},
		{
			// A custom execution keeps its own processor path and output;
			// the implicit default-compile, which has neither, reports
			// nothing.
			name: "custom compiler execution with its own output",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>generate-mappers</id>
                            <goals><goal>compile</goal></goals>
                            <configuration>
                                <generatedSourcesDirectory>${project.build.directory}/mappers</generatedSourcesDirectory>
                                <annotationProcessorPaths>
                                    <path>
                                        <groupId>org.mapstruct</groupId>
                                        <artifactId>mapstruct-processor</artifactId>
                                    </path>
                                </annotationProcessorPaths>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/mappers", compilerPlugin, derivationConfigured),
			},
		},
		{
			name: "processing off for one execution",
			pom: projectPOM(mapstructDependency("compile") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>default-compile</id>
                            <configuration><proc>none</proc></configuration>
                        </execution>
                    </executions>
                </plugin>`)),
			want: annotationOutputs[1:],
		},
		{
			// Binding an execution to the phase none disables it.
			name: "compilation disabled with phase none",
			pom: projectPOM(mapstructDependency("compile") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>default-testCompile</id>
                            <phase>none</phase>
                        </execution>
                    </executions>
                </plugin>`)),
			want: annotationOutputs[:1],
		},
		{
			// With a processor path, javac stops searching the classpath,
			// so MapStruct as a dependency no longer runs.
			name: "processor path without the classpath processor",
			pom: projectPOM(mapstructDependency("compile") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessorPaths>
                            <path>
                                <groupId>org.projectlombok</groupId>
                                <artifactId>lombok</artifactId>
                            </path>
                        </annotationProcessorPaths>
                    </configuration>
                </plugin>`)),
		},
		{
			// An execution's configuration reaches only the compilation
			// that execution runs.
			name: "processor path on the test compilation only",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>default-testCompile</id>
                            <configuration>
                                <annotationProcessorPaths>
                                    <path>
                                        <groupId>com.google.dagger</groupId>
                                        <artifactId>dagger-compiler</artifactId>
                                    </path>
                                </annotationProcessorPaths>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)),
			want: annotationOutputs[1:],
		},
		{
			// An allow-list runs only the classes it names, so MapStruct on
			// the classpath generates nothing beside another processor.
			name: "allow-list without the processor",
			pom: projectPOM(mapstructDependency("compile") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessors>
                            <annotationProcessor>com.example.AuditProcessor</annotationProcessor>
                        </annotationProcessors>
                    </configuration>
                </plugin>`)),
		},
		{
			name: "allow-list naming the processor",
			pom: projectPOM(mapstructDependency("compile") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessors>
                            <annotationProcessor>com.example.AuditProcessor</annotationProcessor>
                            <annotationProcessor>org.mapstruct.ap.MappingProcessor</annotationProcessor>
                        </annotationProcessors>
                    </configuration>
                </plugin>`)),
			want: annotationOutputs,
		},
		{
			// Maven also accepts an array parameter as comma-separated text.
			name: "allow-list as comma-separated text",
			pom: projectPOM(mapstructDependency("compile") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessors>com.example.AuditProcessor, org.mapstruct.ap.MappingProcessor</annotationProcessors>
                    </configuration>
                </plugin>`)),
			want: annotationOutputs,
		},
		{
			name: "main compilation skipped",
			pom: projectPOM(`
    <properties>
        <maven.main.skip>true</maven.main.skip>
    </properties>` + mapstructDependency("compile")),
			want: annotationOutputs[1:],
		},
		{
			name: "test compilation skipped",
			pom: projectPOM(`
    <properties>
        <maven.test.skip>true</maven.test.skip>
    </properties>` + mapstructDependency("compile")),
			want: annotationOutputs[:1],
		},
		{
			// From JDK 23 javac skips classpath discovery unless processing
			// is configured, and a Java 23 module needs such a JDK.
			name: "classpath processor on Java 23",
			pom: projectPOM(`
    <properties>
        <maven.compiler.release>23</maven.compiler.release>
    </properties>` + mapstructDependency("compile")),
		},
		{
			name: "classpath processor on Java 23 with proc full",
			pom: projectPOM(`
    <properties>
        <maven.compiler.release>23</maven.compiler.release>
        <maven.compiler.proc>full</maven.compiler.proc>
    </properties>` + mapstructDependency("compile")),
			want: annotationOutputs,
		},
		{
			name: "classpath processor on Java 23 named in an allow-list",
			pom: projectPOM(`
    <properties>
        <maven.compiler.release>25</maven.compiler.release>
    </properties>` + mapstructDependency("compile") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessors>
                            <annotationProcessor>org.mapstruct.ap.MappingProcessor</annotationProcessor>
                        </annotationProcessors>
                    </configuration>
                </plugin>`)),
			want: annotationOutputs,
		},
		{
			// Below Java 23 the building JDK could be older, and older
			// ones still search the classpath.
			name: "classpath processor on Java 21",
			pom: projectPOM(`
    <properties>
        <maven.compiler.release>21</maven.compiler.release>
    </properties>` + mapstructDependency("compile")),
			want: annotationOutputs,
		},
		{
			// The level an execution compiles for decides, so a module
			// whose main compilation targets 23 loses classpath discovery
			// there and keeps it for tests at 21.
			name: "Java 23 set on one compiler execution",
			pom: projectPOM(`
    <properties>
        <maven.compiler.release>21</maven.compiler.release>
    </properties>` + mapstructDependency("compile") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>default-compile</id>
                            <configuration><release>23</release></configuration>
                        </execution>
                    </executions>
                </plugin>`)),
			want: annotationOutputs[1:],
		},
		{
			// release wins over a lower target, as javac applies it.
			name: "release over target",
			pom: projectPOM(`
    <properties>
        <maven.compiler.target>17</maven.compiler.target>
    </properties>` + mapstructDependency("compile") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration><release>24</release></configuration>
                </plugin>`)),
		},
		{
			// The plain querydsl-apt jar registers no processor; only its
			// classifier jars do, so javac discovers nothing from it.
			name: "QueryDSL without a classifier",
			pom:  projectPOM(querydslDependency("")),
		},
		{
			name: "QueryDSL with the jpa classifier",
			pom:  projectPOM(querydslDependency("jpa")),
			want: annotationOutputs,
		},
		{
			// Naming the processor class makes javac load it from the
			// plain jar, which registers nothing.
			name: "QueryDSL without a classifier, named in an allow-list",
			pom: projectPOM(querydslDependency("") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessors>
                            <annotationProcessor>com.querydsl.apt.jpa.JPAAnnotationProcessor</annotationProcessor>
                        </annotationProcessors>
                    </configuration>
                </plugin>`)),
			want: annotationOutputs,
		},
		{
			name: "QueryDSL on the processor path without a classifier",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessorPaths>
                            <path>
                                <groupId>com.querydsl</groupId>
                                <artifactId>querydsl-apt</artifactId>
                            </path>
                        </annotationProcessorPaths>
                    </configuration>
                </plugin>`)),
		},
		{
			name: "QueryDSL on the processor path with a classifier",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessorPaths>
                            <path>
                                <groupId>io.github.openfeign.querydsl</groupId>
                                <artifactId>querydsl-apt</artifactId>
                                <classifier>jakarta</classifier>
                            </path>
                        </annotationProcessorPaths>
                    </configuration>
                </plugin>`)),
			want: annotationOutputs,
		},
		{
			// A scope may come from a property, directly or through
			// dependencyManagement.
			name: "scope from a property",
			pom: projectPOM(`
    <properties>
        <processor.scope>test</processor.scope>
    </properties>` + mapstructDependency("${processor.scope}")),
			want: annotationOutputs[1:],
		},
		{
			name: "managed scope from a property",
			pom: projectPOM(`
    <properties>
        <processor.scope>provided</processor.scope>
    </properties>
    <dependencyManagement>
        <dependencies>
            <dependency>
                <groupId>org.mapstruct</groupId>
                <artifactId>mapstruct-processor</artifactId>
                <scope>${processor.scope}</scope>
            </dependency>
        </dependencies>
    </dependencyManagement>
    <dependencies>
        <dependency>
            <groupId>org.mapstruct</groupId>
            <artifactId>mapstruct-processor</artifactId>
        </dependency>
    </dependencies>`),
			want: annotationOutputs,
		},
		{
			// Maven rejects a scope it cannot resolve, so it reaches
			// neither compilation.
			name: "unresolvable scope",
			pom:  projectPOM(mapstructDependency("${undefined.scope}")),
		},
		{
			// Maven 4 puts processor-typed dependencies on the processor
			// path, which is explicit configuration javac 23 still honours.
			name: "Maven 4 processor-typed dependency on Java 23",
			pom: projectPOM(`
    <properties>
        <maven.compiler.release>23</maven.compiler.release>
    </properties>
    <dependencies>
        <dependency>
            <groupId>org.mapstruct</groupId>
            <artifactId>mapstruct-processor</artifactId>
            <type>classpath-processor</type>
            <scope>provided</scope>
        </dependency>
    </dependencies>`),
			want: annotationOutputs,
		},
		{
			// Any processor-typed dependency forms a processor path, so
			// javac stops searching the classpath, where MapStruct sits.
			name: "Maven 4 processor path hides a classpath processor",
			pom: projectPOM(`
    <dependencies>
        <dependency>
            <groupId>org.projectlombok</groupId>
            <artifactId>lombok</artifactId>
            <type>processor</type>
            <scope>provided</scope>
        </dependency>
        <dependency>
            <groupId>org.mapstruct</groupId>
            <artifactId>mapstruct-processor</artifactId>
            <scope>provided</scope>
        </dependency>
    </dependencies>`),
		},
		{
			name: "processor coordinates from properties",
			pom: projectPOM(`
    <properties>
        <processor.group>com.querydsl</processor.group>
        <processor.classifier>jpa</processor.classifier>
    </properties>` + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessorPaths>
                            <path>
                                <groupId>${processor.group}</groupId>
                                <artifactId>querydsl-apt</artifactId>
                                <classifier>${processor.classifier}</classifier>
                            </path>
                        </annotationProcessorPaths>
                    </configuration>
                </plugin>`)),
			want: annotationOutputs,
		},
		{
			name: "dependency coordinates and allow-list from properties",
			pom: projectPOM(`
    <properties>
        <mapstruct.group>org.mapstruct</mapstruct.group>
        <mapstruct.class>org.mapstruct.ap.MappingProcessor</mapstruct.class>
    </properties>
    <dependencies>
        <dependency>
            <groupId>${mapstruct.group}</groupId>
            <artifactId>mapstruct-processor</artifactId>
        </dependency>
    </dependencies>` + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <annotationProcessors>
                            <annotationProcessor>${mapstruct.class}</annotationProcessor>
                        </annotationProcessors>
                    </configuration>
                </plugin>`)),
			want: annotationOutputs,
		},
		{
			// An implicit execution can bind the other compiler goal too;
			// Maven runs both with that execution's configuration.
			name: "default-compile also bound to testCompile",
			pom: projectPOM(withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>default-compile</id>
                            <goals><goal>compile</goal><goal>testCompile</goal></goals>
                            <configuration>
                                <annotationProcessorPaths>
                                    <path>
                                        <groupId>org.mapstruct</groupId>
                                        <artifactId>mapstruct-processor</artifactId>
                                    </path>
                                </annotationProcessorPaths>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)),
			want: annotationOutputs,
		},
		{
			name: "test-scoped processor",
			pom:  projectPOM(mapstructDependency("test")),
			want: annotationOutputs[1:],
		},
		{
			// Runtime dependencies miss the main compile classpath but are
			// on the test one.
			name: "runtime-scoped processor reaches only the tests",
			pom:  projectPOM(mapstructDependency("runtime")),
			want: annotationOutputs[1:],
		},
		{
			// Lombok rewrites classes in place and writes no source file.
			name: "lombok alone generates nothing",
			pom: projectPOM(`
    <dependencies>
        <dependency>
            <groupId>org.projectlombok</groupId>
            <artifactId>lombok</artifactId>
        </dependency>
    </dependencies>`),
		},
		{
			name: "annotation processing switched off",
			pom: projectPOM(`
    <properties>
        <maven.compiler.proc>none</maven.compiler.proc>
    </properties>` + mapstructDependency("compile")),
		},
		{
			name: "pom packaging does not compile",
			pom:  projectPOM("<packaging>pom</packaging>" + mapstructDependency("compile")),
		},
		{
			name: "configured generated sources directory",
			pom: projectPOM(mapstructDependency("compile") + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>default-compile</id>
                            <configuration>
                                <generatedSourcesDirectory>${project.build.directory}/apt</generatedSourcesDirectory>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)),
			want: []generatedSource{
				rootSource("target/apt", compilerPlugin, derivationConfigured),
				annotationOutputs[1],
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertSources(t, generatedSourcesIn(t, extractPOM(t, tc.pom)), tc.want)
		})
	}
}

// Generators usually sit in leaf modules below an aggregator that
// declares none, and a module inherits the generators its on-disk parent
// runs. This is the shape of onap/cps-ncmp-dmi-plugin.
func TestGeneratedSourcesAcrossNestedReactor(t *testing.T) {
	generator := withPlugins(`
                <plugin>
                    <groupId>org.openapitools</groupId>
                    <artifactId>openapi-generator-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>generate</goal></goals>
                            <configuration>
                                <configOptions>
                                    <sourceFolder>src/gen/java</sourceFolder>
                                </configOptions>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)
	module := func(artifactID, parent, body string) string {
		return `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>com.example</groupId>
        <artifactId>` + parent + `</artifactId>
        <version>1.0.0</version>
    </parent>
    <artifactId>` + artifactID + `</artifactId>
` + body + `
</project>`
	}

	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules>
        <module>service</module>
        <module>stub</module>
    </modules>`),
		"service/pom.xml":       module("service", "generated", generator),
		"stub/pom.xml":          module("stub", "generated", "<packaging>pom</packaging><modules><module>stub-app</module></modules>"+generator),
		"stub/stub-app/pom.xml": module("stub-app", "stub", ""),
	})

	sourceFolder := "target/generated-sources/openapi/src/gen/java"
	assertSources(t, extractTree(t, root), []generatedSource{
		{Module: "service", Path: sourceFolder, Plugin: openAPI, Derivation: derivationPluginDefault},
		{Module: "stub", Path: sourceFolder, Plugin: openAPI, Derivation: derivationPluginDefault},
		{Module: "stub/stub-app", Path: sourceFolder, Plugin: openAPI, Derivation: derivationPluginDefault},
	})
}

// childPOM is a module POM naming a parent, with an optional relativePath.
func childPOM(artifactID, parentArtifactID, parentVersion, relativePath, body string) string {
	if relativePath != "" {
		relativePath = "<relativePath>" + relativePath + "</relativePath>"
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>com.example</groupId>
        <artifactId>` + parentArtifactID + `</artifactId>
        <version>` + parentVersion + `</version>
        ` + relativePath + `
    </parent>
    <artifactId>` + artifactID + `</artifactId>
` + body + `
</project>`
}

// parentPOM is a pom-packaged parent at the given version.
func parentPOM(artifactID, version, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>` + artifactID + `</artifactId>
    <version>` + version + `</version>
    <packaging>pom</packaging>
` + body + `
</project>`
}

// ONAP cps-events names jsonschema2pojo and leaves the rest to cps-parent:
// the module binds no goal, and the parent's pluginManagement binds it.
// A corporate parent fixing the output there too must win over the
// default.
func TestGeneratedSourcesInheritParentConfiguration(t *testing.T) {
	root := writeTree(t, map[string]string{
		"parent/pom.xml": parentPOM("corporate-parent", "1.0.0", `
    <properties>
        <generated.root>${project.build.directory}/codegen</generated.root>
    </properties>
    <build>
        <pluginManagement>
            <plugins>
                <plugin>
                    <groupId>org.jsonschema2pojo</groupId>
                    <artifactId>jsonschema2pojo-maven-plugin</artifactId>
                    <configuration>
                        <outputDirectory>${generated.root}/json</outputDirectory>
                    </configuration>
                    <executions>
                        <execution>
                            <goals><goal>generate</goal></goals>
                        </execution>
                    </executions>
                </plugin>
            </plugins>
        </pluginManagement>
    </build>`),
		"app/pom.xml": childPOM("app", "corporate-parent", "1.0.0", "../parent/pom.xml", withPlugins(`
                <plugin>
                    <groupId>org.jsonschema2pojo</groupId>
                    <artifactId>jsonschema2pojo-maven-plugin</artifactId>
                </plugin>`)),
	})

	assertSources(t, extractTree(t, filepath.Join(root, "app")), []generatedSource{
		rootSource("target/codegen/json", "jsonschema2pojo-maven-plugin", derivationConfigured),
	})
}

// Maven only takes the POM at relativePath as the parent when its
// coordinates match. An aggregator at the default ../pom.xml that is not
// the declared parent contributes nothing, or its generators would be
// attributed to every module.
func TestGeneratedSourcesIgnoreUndeclaredParent(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules><module>app</module></modules>` + withPlugins(antlrBound)),
		"app/pom.xml": `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>org.example.remote</groupId>
        <artifactId>remote-parent</artifactId>
        <version>1.0.0</version>
    </parent>
    <artifactId>app</artifactId>
</project>`,
	})

	assertSources(t, extractTree(t, root), []generatedSource{antlrDefault})
}

// Maven's readParentLocally matches the local candidate on its groupId,
// its own or inherited from its own parent, as well as its artifactId, so
// a POM with another groupId, or none at all, is not the parent.
func TestGeneratedSourcesCheckParentGroupID(t *testing.T) {
	tests := []struct {
		name    string
		groupID string
		want    []generatedSource
	}{
		{name: "matching groupId", groupID: "<groupId>com.example</groupId>", want: []generatedSource{antlrDefault}},
		{name: "another groupId", groupID: "<groupId>org.example.other</groupId>"},
		{name: "no groupId"},
		{
			name: "inherited groupId",
			groupID: `<parent>
        <groupId>com.example</groupId>
        <artifactId>corporate-root</artifactId>
        <version>1</version>
        <relativePath/>
    </parent>`,
			want: []generatedSource{antlrDefault},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml": `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    ` + tc.groupID + `
    <artifactId>local-parent</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>
` + withPlugins(antlrBound) + `
</project>`,
				"app/pom.xml": childPOM("app", "local-parent", "1.0.0", "", ""),
			})
			assertSources(t, extractTree(t, filepath.Join(root, "app")), tc.want)
		})
	}
}

// A local POM with the parent's coordinates but another version is not
// the parent either: Maven's readParentLocally compares the raw version
// strings and drops a mismatch for the declared version from the
// repository. So a CI-friendly ${revision} matches a parent that declares
// ${revision} too, and nothing else. Maven also accepts a range containing
// the version; the action, without Maven's version ordering, treats a
// range as a mismatch, the safe side, and a missing version too.
func TestGeneratedSourcesCheckParentVersion(t *testing.T) {
	tests := []struct {
		name            string
		parentVersion   string
		declaredVersion string
		want            []generatedSource
	}{
		{name: "matching version", parentVersion: "2.0.0", declaredVersion: "2.0.0", want: []generatedSource{antlrDefault}},
		{name: "version skew", parentVersion: "2.0.0", declaredVersion: "1.0.0"},
		{name: "CI-friendly version on both", parentVersion: "${revision}", declaredVersion: "${revision}", want: []generatedSource{antlrDefault}},
		{name: "CI-friendly version against a literal", parentVersion: "2.0.0", declaredVersion: "${revision}"},
		{name: "version range", parentVersion: "2.0.0", declaredVersion: "[1.0,3.0)"},
		{name: "no declared version", parentVersion: "2.0.0", declaredVersion: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml":     parentPOM("local-parent", tc.parentVersion, withPlugins(antlrBound)),
				"app/pom.xml": childPOM("app", "local-parent", tc.declaredVersion, "", ""),
			})
			assertSources(t, extractTree(t, filepath.Join(root, "app")), tc.want)
		})
	}
}

// Maven combines the goals of executions that share an id, so a module
// adding a goal to an inherited execution keeps the inherited one too.
func TestGeneratedSourcesCombineInheritedGoals(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": parentPOM("local-parent", "1.0.0", withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>schemas</id>
                            <goals><goal>xjc</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
		"app/pom.xml": childPOM("app", "local-parent", "1.0.0", "", withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>schemas</id>
                            <goals><goal>testXjc</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
	})

	assertSources(t, extractTree(t, filepath.Join(root, "app")), []generatedSource{
		rootSource("target/generated-test-sources/jaxb", "jaxb2-maven-plugin", derivationPluginDefault),
		rootSource("target/generated-sources/jaxb", "jaxb2-maven-plugin", derivationPluginDefault),
	})
}

// Maven applies inheritance one level at a time
// (DefaultInheritanceAssembler, MavenModelMerger.mergePlugin_Executions):
// a plugin passes to a child if it is inherited or has executions, its
// configuration only if it is inherited, and each execution by its own
// inherited flag, else the plugin's. Each case runs a grandparent, a
// parent and the scanned module.
func TestGeneratedSourcesFollowMavenInheritance(t *testing.T) {
	jaxbPlugin := func(pluginFlag, config, executions string) string {
		return `
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    ` + pluginFlag + config + `
                    <executions>` + executions + `</executions>
                </plugin>`
	}
	execution := func(id, goal, flag string) string {
		return `<execution><id>` + id + `</id>` + flag + `<goals><goal>` + goal + `</goal></goals></execution>`
	}
	notInherited := "<inherited>false</inherited>"
	inheritedFlag := "<inherited>true</inherited>"
	jaxbMain := rootSource("target/generated-sources/jaxb", "jaxb2-maven-plugin", derivationPluginDefault)

	tests := []struct {
		name        string
		grandparent string
		parent      string
		want        []generatedSource
	}{
		{
			// Previously dropped with the plugin: an execution's own
			// flag overrides the plugin's, but the plugin-level output
			// stays behind with the plugin's configuration.
			name: "explicitly inherited execution under a non-inherited plugin",
			parent: withPlugins(jaxbPlugin(notInherited,
				`<configuration><outputDirectory>${project.build.directory}/parent-only</outputDirectory></configuration>`,
				execution("main", "xjc", inheritedFlag)+execution("tests", "testXjc", ""))),
			want: []generatedSource{jaxbMain},
		},
		{
			// Previously over-reported: the parent's flag stops what it
			// inherited from above, not only its own declaration.
			name:        "parent blocks a grandparent's execution",
			grandparent: withPlugins(jaxbPlugin("", "", execution("main", "xjc", ""))),
			parent:      withPlugins(jaxbPlugin(notInherited, "", "")),
		},
		{
			name:        "explicitly inherited execution passes a blocking parent",
			grandparent: withPlugins(jaxbPlugin("", "", execution("main", "xjc", inheritedFlag))),
			parent:      withPlugins(jaxbPlugin(notInherited, "", "")),
			want:        []generatedSource{jaxbMain},
		},
		{
			// Previously missed: a non-inherited plugin with executions
			// still reaches the child as a shell, and inherited
			// pluginManagement then supplies the execution that runs.
			name: "non-inherited shell still takes managed executions",
			parent: `
    <build>
        <pluginManagement>
            <plugins>` + jaxbPlugin("", "", execution("managed", "xjc", "")) + `</plugins>
        </pluginManagement>
        <plugins>` + jaxbPlugin(notInherited, "", execution("local", "testXjc", "")) + `</plugins>
    </build>`,
			want: []generatedSource{jaxbMain},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml":         parentPOM("top", "1.0.0", tc.grandparent),
				"mid/pom.xml":     childPOM("mid", "top", "1.0.0", "", "<packaging>pom</packaging>"+tc.parent),
				"mid/app/pom.xml": childPOM("app", "mid", "1.0.0", "", ""),
			})
			assertSources(t, extractTree(t, filepath.Join(root, "mid", "app")), tc.want)
		})
	}
}

// Maven withholds a parent's plugin or execution marked
// <inherited>false</inherited> from its children.
func TestGeneratedSourcesHonourInheritedFalse(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": parentPOM("local-parent", "1.0.0", withPlugins(`
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <inherited>false</inherited>
                    <executions>
                        <execution>
                            <goals><goal>antlr4</goal></goals>
                        </execution>
                    </executions>
                </plugin>
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>parent-only</id>
                            <inherited>false</inherited>
                            <goals><goal>xjc</goal></goals>
                        </execution>
                        <execution>
                            <id>shared</id>
                            <goals><goal>testXjc</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
		"app/pom.xml": childPOM("app", "local-parent", "1.0.0", "", ""),
	})

	assertSources(t, extractTree(t, filepath.Join(root, "app")), []generatedSource{
		rootSource("target/generated-test-sources/jaxb", "jaxb2-maven-plugin", derivationPluginDefault),
	})
}

// The flat list carries no module, so a consumer applies every path to
// every module. A path generated in one module but holding hand-written
// sources in another must stay out of it; generated_sources keeps it,
// paired with the module that generates it.
func TestGeneratedSourceDirsSafeAcrossReactor(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules>
        <module>gen</module>
        <module>app</module>
    </modules>`),
		"gen/pom.xml": childPOM("gen", "generated", "1.0.0", "", withPlugins(antlrBound+openAPIExecution("", "", `
                                <output>${project.basedir}</output>
                                <configOptions>
                                    <sourceFolder>src/gen/java</sourceFolder>
                                </configOptions>`))),
		"app/pom.xml": childPOM("app", "generated", "1.0.0", "", `
    <build>
        <sourceDirectory>src/gen/java</sourceDirectory>
    </build>`),
	})

	metadata, err := NewMavenExtractor().Extract(root)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	ls := metadata.LanguageSpecific
	assertSources(t, ls["generated_sources"].([]generatedSource), []generatedSource{
		{Module: "gen", Path: "target/generated-sources/antlr4", Plugin: "antlr4-maven-plugin", Derivation: derivationPluginDefault},
		{Module: "gen", Path: "src/gen/java", Plugin: openAPI, Derivation: derivationConfigured},
	})
	assertStringSlice(t, ls, "generated_source_dirs", []string{"target/generated-sources/antlr4"})
}

// Maven merges inherited dependencies by key, the nearest declaration
// winning, and fills an unset scope from dependencyManagement. A parent's
// compile-scoped processor that the module narrows to test reaches only
// the tests.
func TestGeneratedSourcesResolveDependencyScope(t *testing.T) {
	mapstruct := func(scope string) string {
		if scope != "" {
			scope = "<scope>" + scope + "</scope>"
		}
		return `
        <dependency>
            <groupId>org.mapstruct</groupId>
            <artifactId>mapstruct-processor</artifactId>
            ` + scope + `
        </dependency>`
	}
	tests := []struct {
		name, parent, child string
	}{
		{
			name:   "module redeclares a parent's dependency",
			parent: "<dependencies>" + mapstruct("compile") + "</dependencies>",
			child:  "<dependencies>" + mapstruct("test") + "</dependencies>",
		},
		{
			name:   "scope from dependencyManagement",
			parent: "<dependencyManagement><dependencies>" + mapstruct("test") + "</dependencies></dependencyManagement>",
			child:  "<dependencies>" + mapstruct("") + "</dependencies>",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml":     parentPOM("local-parent", "1.0.0", tc.parent),
				"app/pom.xml": childPOM("app", "local-parent", "1.0.0", "", tc.child),
			})
			assertSources(t, extractTree(t, filepath.Join(root, "app")), annotationOutputs[1:])
		})
	}
}

// combine.self="override" stops Maven merging in configuration from farther
// up the inheritance chain, so a parameter the overriding element omits
// falls back to the plugin default rather than the parent's value.
func TestGeneratedSourcesHonourCombineSelfOverride(t *testing.T) {
	managedAntlr := `
    <build>
        <pluginManagement>
            <plugins>
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <configuration>
                        <outputDirectory>${project.build.directory}/inherited</outputDirectory>
                    </configuration>
                    <executions>
                        <execution>
                            <goals><goal>antlr4</goal></goals>
                        </execution>
                    </executions>
                </plugin>
            </plugins>
        </pluginManagement>
    </build>`
	managedOpenAPI := `
    <build>
        <pluginManagement>
            <plugins>
                <plugin>
                    <groupId>org.openapitools</groupId>
                    <artifactId>openapi-generator-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>api</id>
                            <goals><goal>generate</goal></goals>
                            <configuration>
                                <configOptions>
                                    <sourceFolder>src/gen/java</sourceFolder>
                                </configOptions>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>
            </plugins>
        </pluginManagement>
    </build>`
	childAntlr := func(attribute string) string {
		return withPlugins(`
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <configuration` + attribute + `><listener>true</listener></configuration>
                </plugin>`)
	}
	tests := []struct {
		name, parent, child string
		want                []generatedSource
	}{
		{
			name:   "merged configuration inherits the output",
			parent: managedAntlr,
			child:  childAntlr(""),
			want: []generatedSource{
				rootSource("target/inherited", "antlr4-maven-plugin", derivationConfigured),
			},
		},
		{
			name:   "overriding configuration drops it",
			parent: managedAntlr,
			child:  childAntlr(` combine.self="override"`),
			want:   []generatedSource{antlrDefault},
		},
		{
			name:   "overriding nested element drops what it omits",
			parent: managedOpenAPI,
			child: withPlugins(`
                <plugin>
                    <groupId>org.openapitools</groupId>
                    <artifactId>openapi-generator-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>api</id>
                            <configuration>
                                <configOptions combine.self="override">
                                    <useTags>true</useTags>
                                </configOptions>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`),
			want: []generatedSource{
				rootSource("target/generated-sources/openapi", openAPI, derivationPluginDefault),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml":     parentPOM("local-parent", "1.0.0", tc.parent),
				"app/pom.xml": childPOM("app", "local-parent", "1.0.0", "", tc.child),
			})
			assertSources(t, extractTree(t, filepath.Join(root, "app")), tc.want)
		})
	}
}

// The opposite overlap: a module whose own sources live elsewhere may
// generate into src/main/java/generated, which is safe for it, but inside
// another module's source root the same path would hide whatever
// hand-written files share the package.
func TestGeneratedSourceDirsSafeInsideOtherModules(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules>
        <module>gen</module>
        <module>app</module>
    </modules>`),
		"gen/pom.xml": childPOM("gen", "generated", "1.0.0", "", `
    <build>
        <sourceDirectory>src/java</sourceDirectory>
        <plugins>
            <plugin>
                <groupId>org.codehaus.mojo</groupId>
                <artifactId>jaxb2-maven-plugin</artifactId>
                <executions>
                    <execution>
                        <goals><goal>xjc</goal></goals>
                        <configuration>
                            <outputDirectory>${project.basedir}/src/main/java/generated</outputDirectory>
                        </configuration>
                    </execution>
                </executions>
            </plugin>
        </plugins>
    </build>`),
		"app/pom.xml": childPOM("app", "generated", "1.0.0", "", ""),
	})

	metadata, err := NewMavenExtractor().Extract(root)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	ls := metadata.LanguageSpecific
	assertSources(t, ls["generated_sources"].([]generatedSource), []generatedSource{
		{Module: "gen", Path: "src/main/java/generated", Plugin: "jaxb2-maven-plugin", Derivation: derivationConfigured},
	})
	if dirs, present := ls["generated_source_dirs"]; present {
		t.Errorf("generated_source_dirs = %v, want absent: the path sits inside app's sources", dirs)
	}
}

// The build directory itself is build output too, so a generator writing
// straight into it is reported for a module whose roots are uncertain,
// and kept in the flat list.
func TestGeneratedSourcesIntoTheBuildDirectory(t *testing.T) {
	root := writeTree(t, map[string]string{
		"app/pom.xml": childPOM("app", "remote-parent", "1.0.0", "", withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>xjc</goal></goals>
                            <configuration><outputDirectory>${project.build.directory}</outputDirectory></configuration>
                        </execution>
                    </executions>
                </plugin>`)),
	})

	metadata, err := NewMavenExtractor().Extract(filepath.Join(root, "app"))
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	ls := metadata.LanguageSpecific
	assertSources(t, generatedSourcesIn(t, ls), []generatedSource{
		rootSource("target", "jaxb2-maven-plugin", derivationConfigured),
	})
}

// A parent resolved from a repository may declare source directories this
// action never sees, so for a module inheriting from one only directories
// inside the build output, which no hand-written source occupies, are
// reported. With the whole ancestry on disk, the source directories are
// known and a generator writing elsewhere is reported too.
func TestGeneratedSourcesWithParentOffDisk(t *testing.T) {
	jaxb := withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>build-output</id>
                            <goals><goal>xjc</goal></goals>
                        </execution>
                        <execution>
                            <id>beside-sources</id>
                            <goals><goal>xjc</goal></goals>
                            <configuration>
                                <outputDirectory>${project.basedir}/src/gen/java</outputDirectory>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)
	buildOutput := rootSource("target/generated-sources/jaxb", "jaxb2-maven-plugin", derivationPluginDefault)
	besideSources := rootSource("src/gen/java", "jaxb2-maven-plugin", derivationConfigured)

	tests := []struct {
		name  string
		files map[string]string
		want  []generatedSource
	}{
		{
			name: "parent off disk",
			files: map[string]string{
				"app/pom.xml": childPOM("app", "remote-parent", "1.0.0", "", jaxb),
			},
			want: []generatedSource{buildOutput},
		},
		{
			name: "whole ancestry on disk",
			files: map[string]string{
				"pom.xml":     parentPOM("local-parent", "1.0.0", ""),
				"app/pom.xml": childPOM("app", "local-parent", "1.0.0", "", jaxb),
			},
			want: []generatedSource{buildOutput, besideSources},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, tc.files)
			assertSources(t, extractTree(t, filepath.Join(root, "app")), tc.want)
		})
	}
}

// build-helper-maven-plugin adds source roots during the build, and a
// generator writing inside one would share it with hand-written code.
// Every root it declares counts, even in a profile that may not run; a
// root inside the build directory is generated output registered as a
// root; and a root that will not resolve leaves the module's roots
// unknown, so only build output is reported.
func TestGeneratedSourcesRespectAddedSourceRoots(t *testing.T) {
	buildHelper := func(sources string) string {
		return `
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>build-helper-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>add-source</goal></goals>
                            <configuration>
                                <sources><source>` + sources + `</source></sources>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`
	}
	jaxb := func(output string) string {
		return `
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>xjc</goal></goals>
                            <configuration>
                                <outputDirectory>` + output + `</outputDirectory>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`
	}
	jaxbSource := func(dir string) []generatedSource {
		return []generatedSource{rootSource(dir, "jaxb2-maven-plugin", derivationConfigured)}
	}
	// remapped declares a source root through a property that a profile
	// the action cannot decide, one activated by the JDK, may remap.
	remapped := func(element, profileRoot string) string {
		return projectPOM(`
    <properties><declared.root>src/main/java</declared.root></properties>
    <build>
        <` + element + `>${declared.root}</` + element + `>
        <plugins>` + jaxb("${project.build.directory}/generated-sources/xjc") + `</plugins>
    </build>
    <profiles>
        <profile>
            <id>relocated</id>
            <activation><jdk>[17,)</jdk></activation>
            <properties><declared.root>` + profileRoot + `</declared.root></properties>
        </profile>
    </profiles>`)
	}

	tests := []struct {
		name string
		pom  string
		want []generatedSource
	}{
		{
			name: "generator inside an added root",
			pom:  projectPOM(withPlugins(buildHelper("src/extra/java") + jaxb("${project.basedir}/src/extra/java/model"))),
		},
		{
			name: "added root declared only in a profile",
			pom: projectPOM(withPlugins(jaxb("${project.basedir}/src/extra/java")) + `
    <profiles>
        <profile>
            <id>extra</id>
            <build><plugins>` + buildHelper("src/extra/java") + `</plugins></build>
        </profile>
    </profiles>`),
		},
		{
			// The common pattern for a generator that registers no root of
			// its own: build-helper adds its output under target.
			name: "added root inside the build directory",
			pom: projectPOM(withPlugins(buildHelper("${project.build.directory}/generated-sources/xjc") +
				jaxb("${project.build.directory}/generated-sources/xjc"))),
			want: jaxbSource("target/generated-sources/xjc"),
		},
		{
			name: "unresolvable added root, generator beside the sources",
			pom:  projectPOM(withPlugins(buildHelper("${undefined.root}/java") + jaxb("${project.basedir}/generated"))),
		},
		{
			name: "unresolvable added root, generator in the build output",
			pom:  projectPOM(withPlugins(buildHelper("${undefined.root}/java") + jaxb("${project.build.directory}/jaxb"))),
			want: jaxbSource("target/jaxb"),
		},
		{
			name: "unresolvable source directory, generator beside the sources",
			pom: projectPOM(`
    <build>
        <sourceDirectory>${undefined.root}/java</sourceDirectory>
        <plugins>` + jaxb("${project.basedir}/generated") + `</plugins>
    </build>`),
		},
		{
			name: "resolvable roots, generator beside the sources",
			pom:  projectPOM(withPlugins(buildHelper("src/extra/java") + jaxb("${project.basedir}/generated"))),
			want: jaxbSource("generated"),
		},
		{
			// Only an added root inside the build directory is build
			// output; a declared source directory there is still
			// hand-written, so a generator inside it is refused.
			name: "declared source directory inside the build directory",
			pom: projectPOM(`
    <build>
        <directory>${project.basedir}/src</directory>
        <plugins>` + jaxb("${project.basedir}/src/main/java/generated") + `</plugins>
    </build>`),
		},
		{
			// A profile whose activation is unknown may turn the declared
			// root into the generator's output, so either reading counts.
			name: "source directory a profile may remap onto the output",
			pom:  remapped("sourceDirectory", "${project.build.directory}/generated-sources/xjc"),
		},
		{
			name: "test source directory a profile may remap onto the output",
			pom:  remapped("testSourceDirectory", "${project.build.directory}/generated-sources/xjc"),
		},
		{
			name: "source directory a profile may remap elsewhere",
			pom:  remapped("sourceDirectory", "src/alt/java"),
			want: jaxbSource("target/generated-sources/xjc"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertSources(t, generatedSourcesIn(t, extractPOM(t, tc.pom)), tc.want)
		})
	}
}

// A backslash separates path elements on every platform, since a POM
// written on Windows may be read on any runner. Otherwise a backslash
// escape out of the module, or a backslash path into a source directory,
// would pass as a single harmless-looking name.
func TestGeneratedSourcesNormaliseBackslashes(t *testing.T) {
	jaxb := func(output string) string {
		return withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>xjc</goal></goals>
                            <configuration>
                                <outputDirectory>` + output + `</outputDirectory>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)
	}
	tests := []struct {
		name string
		pom  string
		want []generatedSource
	}{
		{name: "escape out of the module", pom: projectPOM(jaxb(`${project.basedir}\..\shared\jaxb`))},
		// Maven joins strings, so with no separator this names a sibling of
		// the module, modulegenerated, not a directory inside it.
		{name: "basedir without a separator", pom: projectPOM(jaxb(`${project.basedir}generated`))},
		{name: "UNC path", pom: projectPOM(jaxb(`\\server\share\jaxb`))},
		{name: "inside a source directory", pom: projectPOM(jaxb(`${project.basedir}\src\main\java\model`))},
		{
			name: "inside a backslash source directory",
			pom: projectPOM(`
    <build>
        <sourceDirectory>src\java</sourceDirectory>
        <plugins>` + `
            <plugin>
                <groupId>org.codehaus.mojo</groupId>
                <artifactId>jaxb2-maven-plugin</artifactId>
                <executions>
                    <execution>
                        <goals><goal>xjc</goal></goals>
                        <configuration>
                            <outputDirectory>${project.basedir}/src/java/model</outputDirectory>
                        </configuration>
                    </execution>
                </executions>
            </plugin>
        </plugins>
    </build>`),
		},
		{
			name: "within the module",
			pom:  projectPOM(jaxb(`${project.build.directory}\generated-sources\xjc`)),
			want: []generatedSource{
				rootSource("target/generated-sources/xjc", "jaxb2-maven-plugin", derivationConfigured),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertSources(t, generatedSourcesIn(t, extractPOM(t, tc.pom)), tc.want)
		})
	}
}

// Copilot's case: module gen builds into out, so out/generated is its build
// output, but module remote inherits from a parent off disk whose source
// roots could include out/generated. A flat path has to be build output
// in every module whose roots are not all known; generated_sources keeps
// the entry against gen.
func TestGeneratedSourceDirsBuildOutputEverywhereUncertain(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules>
        <module>gen</module>
        <module>remote</module>
    </modules>`),
		"gen/pom.xml": childPOM("gen", "remote-parent", "1.0.0", "", `
    <build>
        <directory>${project.basedir}/out</directory>
        <plugins>`+antlrBound+`</plugins>
    </build>`),
		"remote/pom.xml": childPOM("remote", "remote-parent", "1.0.0", "", ""),
	})

	metadata, err := NewMavenExtractor().Extract(root)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	ls := metadata.LanguageSpecific
	assertSources(t, ls["generated_sources"].([]generatedSource), []generatedSource{
		{Module: "gen", Path: "out/generated-sources/antlr4", Plugin: "antlr4-maven-plugin", Derivation: derivationPluginDefault},
	})
	if dirs, present := ls["generated_source_dirs"]; present {
		t.Errorf("generated_source_dirs = %v, want absent: out is not remote's build directory", dirs)
	}
}

// Windows and macOS file systems usually ignore case, so a path differing
// from a source directory only by case may be the same directory.
func TestGeneratedSourcesOverlapIgnoresCase(t *testing.T) {
	pom := projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>xjc</goal></goals>
                            <configuration>
                                <outputDirectory>${project.basedir}/SRC/Main/Java/generated</outputDirectory>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`))
	assertSources(t, generatedSourcesIn(t, extractPOM(t, pom)), nil)
}

// Maven reads <inherited> as Boolean.parseBoolean: absent means inherited,
// and any present value but true, in any case, withholds the plugin.
func TestGeneratedSourcesReadInheritedAsMavenDoes(t *testing.T) {
	tests := []struct {
		flag string
		want []generatedSource
	}{
		{flag: "", want: []generatedSource{antlrDefault}},
		{flag: "<inherited>TRUE</inherited>", want: []generatedSource{antlrDefault}},
		{flag: "<inherited>FALSE</inherited>"},
		{flag: "<inherited>no</inherited>"},
		// parseBoolean("") is false: a present but empty flag withholds.
		{flag: "<inherited/>"},
	}
	for _, tc := range tests {
		t.Run(tc.flag, func(t *testing.T) {
			root := writeTree(t, map[string]string{
				"pom.xml": parentPOM("local-parent", "1.0.0", withPlugins(`
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    `+tc.flag+`
                    <executions>
                        <execution>
                            <goals><goal>antlr4</goal></goals>
                        </execution>
                    </executions>
                </plugin>`)),
				"app/pom.xml": childPOM("app", "local-parent", "1.0.0", "", ""),
			})
			assertSources(t, extractTree(t, filepath.Join(root, "app")), tc.want)
		})
	}
}

// Maven puts no limit on how deep a parent chain runs. A generator
// declared twelve parents up still reaches the module; a chain whose
// relativePaths loop back is stopped where a parent repeats.
func TestGeneratedSourcesThroughDeepParentChains(t *testing.T) {
	const depth = 12
	files := map[string]string{}
	dir := ""
	for level := depth; level >= 1; level-- {
		body := ""
		if level == depth {
			body = withPlugins(antlrBound)
		}
		pom := parentPOM(fmt.Sprintf("level%d", level), "1.0.0", body)
		if level < depth {
			pom = childPOM(fmt.Sprintf("level%d", level), fmt.Sprintf("level%d", level+1), "1.0.0", "", "<packaging>pom</packaging>")
		}
		files[path.Join(dir, "pom.xml")] = pom
		dir = path.Join(dir, fmt.Sprintf("l%d", level))
	}
	files[path.Join(dir, "pom.xml")] = childPOM("app", "level1", "1.0.0", "", "")
	root := writeTree(t, files)

	assertSources(t, extractTree(t, filepath.Join(root, filepath.FromSlash(dir))), []generatedSource{antlrDefault})

	loop := writeTree(t, map[string]string{
		"a/pom.xml": childPOM("a", "b", "1.0.0", "../b/pom.xml", withPlugins(antlrBound)),
		"b/pom.xml": childPOM("b", "a", "1.0.0", "../a/pom.xml", ""),
	})
	assertSources(t, extractTree(t, filepath.Join(loop, "a")), []generatedSource{antlrDefault})
}

// Maven interpolates a plugin's coordinates and its executions' ids,
// phases and goals before deciding what runs, but assembles inheritance
// on the POMs as written. So property-backed identity is recognised, a
// phase resolving to none disables, and an <inherited> flag written as a
// property is read raw, by Boolean.parseBoolean, as false.
func TestGeneratedSourcesInterpolatePluginModel(t *testing.T) {
	propertyPlugin := projectPOM(`
    <properties>
        <generator.group>org.antlr</generator.group>
        <generator.goal>antlr4</generator.goal>
        <generator.phase>generate-sources</generator.phase>
    </properties>` + withPlugins(`
                <plugin>
                    <groupId>${generator.group}</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <phase>${generator.phase}</phase>
                            <goals><goal>${generator.goal}</goal></goals>
                        </execution>
                    </executions>
                </plugin>`))
	disabledByProperty := projectPOM(`
    <properties><generation.phase>none</generation.phase></properties>` + withPlugins(`
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <phase>${generation.phase}</phase>
                            <goals><goal>antlr4</goal></goals>
                        </execution>
                    </executions>
                </plugin>`))

	assertSources(t, generatedSourcesIn(t, extractPOM(t, propertyPlugin)), []generatedSource{antlrDefault})
	assertSources(t, generatedSourcesIn(t, extractPOM(t, disabledByProperty)), nil)

	// A phase that keeps a property unresolved names no lifecycle phase,
	// so Maven never schedules the execution: neither a generator's nor
	// the compiler's.
	unresolvedPhase := projectPOM(withPlugins(`
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <phase>${undefined.phase}</phase>
                            <goals><goal>antlr4</goal></goals>
                        </execution>
                    </executions>
                </plugin>`))
	assertSources(t, generatedSourcesIn(t, extractPOM(t, unresolvedPhase)), nil)
	unresolvedCompilePhase := projectPOM(`
    <dependencies>
        <dependency>
            <groupId>org.mapstruct</groupId>
            <artifactId>mapstruct-processor</artifactId>
        </dependency>
    </dependencies>` + withPlugins(`
                <plugin>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>default-compile</id>
                            <phase>${undefined.phase}</phase>
                        </execution>
                    </executions>
                </plugin>`))
	assertSources(t, generatedSourcesIn(t, extractPOM(t, unresolvedCompilePhase)), annotationOutputs[1:])

	inheritedFlag := writeTree(t, map[string]string{
		"pom.xml": parentPOM("local-parent", "1.0.0", `
    <properties><share.generator>true</share.generator></properties>`+withPlugins(`
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <inherited>${share.generator}</inherited>
                    <executions>
                        <execution><goals><goal>antlr4</goal></goals></execution>
                    </executions>
                </plugin>`)),
		"app/pom.xml": childPOM("app", "local-parent", "1.0.0", "", ""),
	})
	assertSources(t, extractTree(t, filepath.Join(inheritedFlag, "app")), nil)

	// Management joins the build plugins after interpolation, so a managed
	// entry written with a property still configures the plugin.
	managedByProperty := projectPOM(`
    <properties><generator.group>org.antlr</generator.group></properties>
    <build>
        <pluginManagement>
            <plugins>
                <plugin>
                    <groupId>${generator.group}</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <executions>
                        <execution><goals><goal>antlr4</goal></goals></execution>
                    </executions>
                </plugin>
            </plugins>
        </pluginManagement>
        <plugins>
            <plugin>
                <groupId>org.antlr</groupId>
                <artifactId>antlr4-maven-plugin</artifactId>
            </plugin>
        </plugins>
    </build>`)
	assertSources(t, generatedSourcesIn(t, extractPOM(t, managedByProperty)), []generatedSource{antlrDefault})
}

// Maven's model interpolator resolves ${project.*} and ${pom.*} from the
// effective model ahead of the POM properties, coordinates inherited from
// the parent and build paths from the Super POM's defaults, and the
// unprefixed model last of all.
func TestGeneratedSourcesResolveModelReferences(t *testing.T) {
	jaxbInto := func(output string) string {
		return withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>xjc</goal></goals>
                            <configuration><outputDirectory>` + output + `</outputDirectory></configuration>
                        </execution>
                    </executions>
                </plugin>`)
	}
	jaxbAt := func(dir string) []generatedSource {
		return []generatedSource{rootSource(dir, "jaxb2-maven-plugin", derivationConfigured)}
	}
	single := []struct {
		name, pom string
		want      []generatedSource
	}{
		{
			name: "artifactId and finalName",
			pom:  projectPOM(jaxbInto("${project.build.directory}/${project.artifactId}/${project.build.finalName}")),
			want: jaxbAt("target/generated/generated-1.0.0"),
		},
		{
			name: "pom. alias and Super POM output directory",
			pom:  projectPOM(jaxbInto("${pom.build.outputDirectory}/../gen-${pom.packaging}")),
			want: jaxbAt("target/gen-jar"),
		},
		{
			// The model wins over a POM property of the same name.
			name: "model ahead of a same-named property",
			pom: projectPOM(`
    <properties><project.artifactId>shadowed</project.artifactId></properties>` +
				jaxbInto("${project.build.directory}/${project.artifactId}")),
			want: jaxbAt("target/generated"),
		},
		{
			// Unprefixed, the model comes last, behind the properties.
			name: "unprefixed model behind a property",
			pom: projectPOM(`
    <properties><artifactId>from-property</artifactId></properties>` +
				jaxbInto("${project.build.directory}/${artifactId}-${version}")),
			want: jaxbAt("target/from-property-1.0.0"),
		},
	}
	for _, tc := range single {
		t.Run(tc.name, func(t *testing.T) {
			assertSources(t, generatedSourcesIn(t, extractPOM(t, tc.pom)), tc.want)
		})
	}

	t.Run("version and groupId inherited", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"pom.xml": parentPOM("local-parent", "2.1.0", ""),
			"app/pom.xml": childPOM("app", "local-parent", "2.1.0", "",
				jaxbInto("${project.build.directory}/${project.groupId}-${project.version}")),
		})
		assertSources(t, extractTree(t, filepath.Join(root, "app")), jaxbAt("target/com.example-2.1.0"))
	})
}

// Maven interpolates <module> entries, so a module named through a
// property is part of the reactor, not an unreadable one.
func TestGeneratedSourcesFromInterpolatedModule(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <properties><service.module>service</service.module></properties>
    <modules><module>${service.module}</module></modules>`),
		"service/pom.xml": childPOM("service", "generated", "1.0.0", "", withPlugins(antlrBound)),
	})

	metadata, err := NewMavenExtractor().Extract(root)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	ls := metadata.LanguageSpecific
	assertSources(t, ls["generated_sources"].([]generatedSource), []generatedSource{
		{Module: "service", Path: "target/generated-sources/antlr4", Plugin: "antlr4-maven-plugin", Derivation: derivationPluginDefault},
	})
	assertStringSlice(t, ls, "generated_source_dirs", []string{"target/generated-sources/antlr4"})
}

// The flat list applies across the reactor, so one module inheriting from
// a parent off disk leaves every module's paths outside the build output
// out of it: that module's source directories are not all known.
func TestGeneratedSourceDirsWithAncestryOffDisk(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules>
        <module>gen</module>
        <module>remote</module>
    </modules>`),
		"gen/pom.xml": childPOM("gen", "generated", "1.0.0", "", withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <id>build-output</id>
                            <goals><goal>xjc</goal></goals>
                        </execution>
                        <execution>
                            <id>beside-sources</id>
                            <goals><goal>xjc</goal></goals>
                            <configuration>
                                <outputDirectory>${project.basedir}/generated</outputDirectory>
                            </configuration>
                        </execution>
                    </executions>
                </plugin>`)),
		"remote/pom.xml": childPOM("remote", "remote-parent", "1.0.0", "", ""),
	})

	metadata, err := NewMavenExtractor().Extract(root)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	ls := metadata.LanguageSpecific
	assertSources(t, ls["generated_sources"].([]generatedSource), []generatedSource{
		{Module: "gen", Path: "target/generated-sources/jaxb", Plugin: "jaxb2-maven-plugin", Derivation: derivationPluginDefault},
		{Module: "gen", Path: "generated", Plugin: "jaxb2-maven-plugin", Derivation: derivationConfigured},
	})
	assertStringSlice(t, ls, "generated_source_dirs", []string{"target/generated-sources/jaxb"})
}

// A module the walk cannot read may hold source roots anywhere, so no
// path is safe to publish in the flat list; generated_sources keeps every
// readable module's entries.
func TestGeneratedSourceDirsWithUnreadableModule(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules>
        <module>gen</module>
        <module>broken</module>
        <module>missing</module>
    </modules>`),
		"gen/pom.xml":    childPOM("gen", "generated", "1.0.0", "", withPlugins(antlrBound)),
		"broken/pom.xml": "<project><unclosed>",
	})

	metadata, err := NewMavenExtractor().Extract(root)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	ls := metadata.LanguageSpecific
	assertSources(t, ls["generated_sources"].([]generatedSource), []generatedSource{
		{Module: "gen", Path: "target/generated-sources/antlr4", Plugin: "antlr4-maven-plugin", Derivation: derivationPluginDefault},
	})
	if dirs, present := ls["generated_source_dirs"]; present {
		t.Errorf("generated_source_dirs = %v, want absent: two modules could not be read", dirs)
	}
}

// profilePOM is a jar POM whose base build declares baseBuild and whose
// profiles section holds profiles.
func profilePOM(baseBody, profiles string) string {
	return projectPOM(baseBody + "\n    <profiles>" + profiles + "\n    </profiles>")
}

// profile is one <profile> with the given activation and body.
func profile(id, activation, body string) string {
	if activation != "" {
		activation = "<activation>" + activation + "</activation>"
	}
	return "\n        <profile><id>" + id + "</id>" + activation + body + "</profile>"
}

// Maven merges active profiles into their POM before anything else reads
// it, so what an active profile adds counts and what an inactive one adds
// does not. Activation follows DefaultProfileSelector for a build with no
// -P and no -D: conditions must all hold; activeByDefault applies only when
// no other profile is activated; and a condition on the JDK, the OS or a
// property the command line may set leaves the profile unknown, so it is
// left out.
func TestGeneratedSourcesFromActiveProfiles(t *testing.T) {
	byDefault := "<activeByDefault>true</activeByDefault>"
	antlrBuild := withPlugins(antlrBound)
	jaxbBuild := withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution><goals><goal>xjc</goal></goals></execution>
                    </executions>
                </plugin>`)
	jaxbDefault := rootSource("target/generated-sources/jaxb", "jaxb2-maven-plugin", derivationPluginDefault)
	antlrOutput := func(dir string) string {
		return withPlugins(`
                <plugin>
                    <groupId>org.antlr</groupId>
                    <artifactId>antlr4-maven-plugin</artifactId>
                    <configuration><outputDirectory>` + dir + `</outputDirectory></configuration>
                    <executions>
                        <execution><goals><goal>antlr4</goal></goals></execution>
                    </executions>
                </plugin>`)
	}

	tests := []struct {
		name string
		pom  string
		want []generatedSource
	}{
		{
			name: "activeByDefault profile adds a generator",
			pom:  profilePOM("", profile("gen", byDefault, antlrBuild)),
			want: []generatedSource{antlrDefault},
		},
		{
			// Maven reads the flag as Boolean.valueOf.
			name: "activeByDefault in upper case",
			pom:  profilePOM("", profile("gen", "<activeByDefault>TRUE</activeByDefault>", antlrBuild)),
			want: []generatedSource{antlrDefault},
		},
		{
			// A value Maven reads as false must not stop the POM parsing.
			name: "activeByDefault that is not a boolean",
			pom:  profilePOM("", profile("gen", "<activeByDefault>${flag}</activeByDefault>", antlrBuild)),
		},
		{
			name: "profile with no activation",
			pom:  profilePOM("", profile("gen", "", antlrBuild)),
		},
		{
			// A sibling activated by its own condition displaces the
			// default profile; pom.xml always exists beside the POM.
			name: "activeByDefault displaced by an activated sibling",
			pom: profilePOM("", profile("gen", byDefault, antlrBuild)+
				profile("xml", "<file><exists>pom.xml</exists></file>", jaxbBuild)),
			want: []generatedSource{jaxbDefault},
		},
		{
			// A sibling that may activate on the JDK may displace it.
			name: "activeByDefault beside a JDK-activated sibling",
			pom: profilePOM("", profile("gen", byDefault, antlrBuild)+
				profile("modern", "<jdk>[21,)</jdk>", "")),
		},
		{
			name: "file condition: missing file",
			pom:  profilePOM("", profile("gen", "<file><missing>${basedir}/absent</missing></file>", antlrBuild)),
			want: []generatedSource{antlrDefault},
		},
		{
			name: "file condition: absent file",
			pom:  profilePOM("", profile("gen", "<file><exists>src/main/proto</exists></file>", antlrBuild)),
		},
		{
			// Conditions must all hold; the OS one cannot be settled.
			name: "file condition with an OS condition",
			pom: profilePOM("", profile("gen",
				"<file><exists>pom.xml</exists></file><os><family>unix</family></os>", antlrBuild)),
		},
		{
			// A path naming a property the command line or environment may
			// set is unknown, not absent, so the default profile beside it
			// may be displaced.
			name: "activeByDefault beside a file condition on an unseen property",
			pom: profilePOM("", profile("gen", byDefault, antlrBuild)+
				profile("user", "<file><exists>${user.dir}/pom.xml</exists></file>", "")),
		},
		{
			// Maven sets the packaging property from the module.
			name: "property condition on packaging",
			pom: profilePOM("", profile("gen",
				"<property><name>packaging</name><value>jar</value></property>", antlrBuild)),
			want: []generatedSource{antlrDefault},
		},
		{
			name: "property condition the command line may set",
			pom:  profilePOM("", profile("gen", "<property><name>!skipGeneration</name></property>", antlrBuild)),
		},
		{
			// The profile wins over the POM's own properties.
			name: "active profile property moves the output",
			pom: profilePOM(`
    <properties><grammar.dir>${project.build.directory}/base</grammar.dir></properties>`+
				antlrOutput("${grammar.dir}"),
				profile("gen", byDefault,
					"<properties><grammar.dir>${project.build.directory}/profile</grammar.dir></properties>")),
			want: []generatedSource{
				rootSource("target/profile", "antlr4-maven-plugin", derivationConfigured),
			},
		},
		{
			name: "active profile plugin configuration wins",
			pom: profilePOM(antlrOutput("${project.build.directory}/base"),
				profile("gen", byDefault, antlrOutput("${project.build.directory}/profile"))),
			want: []generatedSource{
				rootSource("target/profile", "antlr4-maven-plugin", derivationConfigured),
			},
		},
		{
			name: "active profile moves the build directory",
			pom: profilePOM(antlrBuild,
				profile("gen", byDefault, "<build><directory>${project.basedir}/out</directory></build>")),
			want: []generatedSource{
				rootSource("out/generated-sources/antlr4", "antlr4-maven-plugin", derivationPluginDefault),
			},
		},
		{
			name: "active profile adds a processor dependency",
			pom: profilePOM("", profile("gen", byDefault, `
            <dependencies>
                <dependency>
                    <groupId>org.mapstruct</groupId>
                    <artifactId>mapstruct-processor</artifactId>
                </dependency>
            </dependencies>`)),
			want: annotationOutputs,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertSources(t, generatedSourcesIn(t, extractPOM(t, tc.pom)), tc.want)
		})
	}
}

// Maven evaluates a parent's profiles once per module it builds, against
// that module's directory, so a parent can switch a generator on only
// where its input exists.
func TestGeneratedSourcesFromParentProfilePerModule(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": parentPOM("local-parent", "1.0.0", `
    <modules>
        <module>with-proto</module>
        <module>without-proto</module>
    </modules>
    <profiles>
        <profile>
            <id>protobuf</id>
            <activation><file><exists>src/main/proto</exists></file></activation>
            <build>
                <plugins>
                    <plugin>
                        <groupId>org.xolstice.maven.plugins</groupId>
                        <artifactId>protobuf-maven-plugin</artifactId>
                        <executions>
                            <execution><goals><goal>compile</goal></goals></execution>
                        </executions>
                    </plugin>
                </plugins>
            </build>
        </profile>
    </profiles>`),
		"with-proto/pom.xml":                  childPOM("with-proto", "local-parent", "1.0.0", "", ""),
		"with-proto/src/main/proto/api.proto": `syntax = "proto3";`,
		"without-proto/pom.xml":               childPOM("without-proto", "local-parent", "1.0.0", "", ""),
	})

	assertSources(t, extractTree(t, root), []generatedSource{
		{Module: "with-proto", Path: "target/generated-sources/protobuf/java",
			Plugin: "protobuf-maven-plugin", Derivation: derivationPluginDefault},
	})
}

// A module that an active profile adds is part of the build, so the walk
// visits it in full.
func TestGeneratedSourcesFromActiveProfileModule(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <profiles>
        <profile>
            <id>extras</id>
            <activation><activeByDefault>true</activeByDefault></activation>
            <modules><module>extra</module></modules>
        </profile>
    </profiles>`),
		"extra/pom.xml": childPOM("extra", "generated", "1.0.0", "", withPlugins(antlrBound)),
	})

	assertSources(t, extractTree(t, root), []generatedSource{
		{Module: "extra", Path: "target/generated-sources/antlr4",
			Plugin: "antlr4-maven-plugin", Derivation: derivationPluginDefault},
	})
}

// A file condition is never settled through a link out of the workspace,
// so a crafted repository cannot probe the runner's file system through
// which profiles the action reports.
func TestProfileFileConditionStaysInWorkspace(t *testing.T) {
	base := writeTree(t, map[string]string{"outside/marker": "x"})
	workspace := filepath.Join(base, "workspace")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	if err := os.Symlink(filepath.Join(base, "outside"), filepath.Join(workspace, "out-link")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if err := os.Symlink(filepath.Join(base, "nowhere"), filepath.Join(workspace, "dangling")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	t.Setenv("GITHUB_WORKSPACE", workspace)

	for _, condition := range []string{
		"<exists>out-link/marker</exists>",
		"<missing>out-link/absent</missing>",
		"<missing>dangling</missing>",
		"<exists>../outside/marker</exists>",
	} {
		t.Run(condition, func(t *testing.T) {
			pom := profilePOM("", profile("probe", "<file>"+condition+"</file>", withPlugins(antlrBound)))
			if err := os.WriteFile(filepath.Join(workspace, "pom.xml"), []byte(pom), 0644); err != nil {
				t.Fatalf("failed to write pom.xml: %v", err)
			}
			assertSources(t, extractTree(t, workspace), nil)
		})
	}
}

// Maven has interpolated a file condition with the declaring POM's own
// properties since 3.2 (FileProfileActivator, then ProfileActivationFile
// PathInterpolator, fed DefaultModelBuilder's raw model properties). It
// uses only that POM's own: a property it merely inherits leaves the
// condition unresolved, and so unknown.
func TestProfileFileConditionUsesOwnProperties(t *testing.T) {
	condition := profile("gen", "<file><exists>${marker}</exists></file>", withPlugins(antlrBound))
	ownProperty := profilePOM("<properties><marker>pom.xml</marker></properties>", condition)
	assertSources(t, generatedSourcesIn(t, extractPOM(t, ownProperty)), []generatedSource{antlrDefault})

	root := writeTree(t, map[string]string{
		"pom.xml": parentPOM("local-parent", "1.0.0", "<properties><marker>pom.xml</marker></properties>"),
		"app/pom.xml": childPOM("app", "local-parent", "1.0.0", "",
			"<profiles>"+condition+"</profiles>"),
	})
	assertSources(t, extractTree(t, filepath.Join(root, "app")), nil)
}

// A profile's own properties merge over its POM's when it activates, so
// what an inactive profile declares is read with them, not only with the
// properties the active profiles bring. Here the active profile and the
// inactive one define root differently: the inactive one's build-helper
// root and module must still guard the flat list.
func TestProfileContentReadWithItsOwnProperties(t *testing.T) {
	jaxbInto := func(output string) string {
		return withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>jaxb2-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>xjc</goal></goals>
                            <configuration><outputDirectory>` + output + `</outputDirectory></configuration>
                        </execution>
                    </executions>
                </plugin>`)
	}
	buildHelper := withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>build-helper-maven-plugin</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>add-source</goal></goals>
                            <configuration><sources><source>${root}</source></sources></configuration>
                        </execution>
                    </executions>
                </plugin>`)

	t.Run("build-helper root", func(t *testing.T) {
		pom := profilePOM(jaxbInto("${project.basedir}/src/extra/java/model"),
			profile("active", "<activeByDefault>true</activeByDefault>", "<properties><root>src/other</root></properties>")+
				profile("extra", "", "<properties><root>src/extra/java</root></properties>"+buildHelper))
		assertSources(t, generatedSourcesIn(t, extractPOM(t, pom)), nil)
	})

	// A property only the profile defines does not resolve alongside the
	// active profiles, but that is not a context the build can take, so
	// the module's roots stay known and output beside them is reported.
	t.Run("property only the profile defines", func(t *testing.T) {
		pom := profilePOM(jaxbInto("${project.basedir}/generated"),
			profile("extra", "", "<properties><root>src/extra/java</root></properties>"+buildHelper))
		assertSources(t, generatedSourcesIn(t, extractPOM(t, pom)), []generatedSource{
			rootSource("generated", "jaxb2-maven-plugin", derivationConfigured),
		})
	})

	// The plugin's identity is a declaration like any other: an inactive
	// profile naming build-helper through its own property still adds a
	// root when it activates.
	t.Run("build-helper named through the profile's property", func(t *testing.T) {
		helper := withPlugins(`
                <plugin>
                    <groupId>org.codehaus.mojo</groupId>
                    <artifactId>${helper.plugin}</artifactId>
                    <executions>
                        <execution>
                            <goals><goal>add-source</goal></goals>
                            <configuration><sources><source>${root}</source></sources></configuration>
                        </execution>
                    </executions>
                </plugin>`)
		pom := profilePOM(jaxbInto("${project.basedir}/src/extra/java/model"),
			profile("extra", "", `<properties>
                <helper.plugin>build-helper-maven-plugin</helper.plugin>
                <root>src/extra/java</root>
            </properties>`+helper))
		assertSources(t, generatedSourcesIn(t, extractPOM(t, pom)), nil)
	})

	t.Run("module", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <build><sourceDirectory>src/root</sourceDirectory></build>
    <modules><module>gen</module></modules>
    <profiles>
        <profile>
            <id>active</id>
            <activation><activeByDefault>true</activeByDefault></activation>
            <properties><extra.module>gen</extra.module></properties>
        </profile>
        <profile>
            <id>extra</id>
            <properties><extra.module>extra</extra.module></properties>
            <modules><module>${extra.module}</module></modules>
        </profile>
    </profiles>`),
			"gen/pom.xml": childPOM("gen", "generated", "1.0.0", "", `
    <build>
        <sourceDirectory>src/java</sourceDirectory>
        <plugins>
            <plugin>
                <groupId>org.codehaus.mojo</groupId>
                <artifactId>jaxb2-maven-plugin</artifactId>
                <executions>
                    <execution>
                        <goals><goal>xjc</goal></goals>
                        <configuration>
                            <outputDirectory>${project.basedir}/src/main/java/generated</outputDirectory>
                        </configuration>
                    </execution>
                </executions>
            </plugin>
        </plugins>
    </build>`),
			"extra/pom.xml": projectPOM(""),
		})

		metadata, err := NewMavenExtractor().Extract(root)
		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}
		if dirs, present := metadata.LanguageSpecific["generated_source_dirs"]; present {
			t.Errorf("generated_source_dirs = %v, want absent: the path sits inside extra's sources", dirs)
		}
	})
}

// A module that only a profile not known to be active declares may join
// the build, and its sources sit in the workspace either way, so its
// source roots guard the flat list too. Its generators are not reported:
// the default build may not run them.
func TestGeneratedSourceDirsCountProfileModules(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <build><sourceDirectory>src/root</sourceDirectory></build>
    <modules><module>gen</module></modules>
    <profiles>
        <profile>
            <id>extras</id>
            <modules><module>extra</module></modules>
        </profile>
    </profiles>`),
		"gen/pom.xml": childPOM("gen", "generated", "1.0.0", "", `
    <build>
        <sourceDirectory>src/java</sourceDirectory>
        <plugins>
            <plugin>
                <groupId>org.codehaus.mojo</groupId>
                <artifactId>jaxb2-maven-plugin</artifactId>
                <executions>
                    <execution>
                        <goals><goal>xjc</goal></goals>
                        <configuration>
                            <outputDirectory>${project.basedir}/src/main/java/generated</outputDirectory>
                        </configuration>
                    </execution>
                </executions>
            </plugin>
        </plugins>
    </build>`),
		"extra/pom.xml": projectPOM(withPlugins(antlrBound)),
	})

	metadata, err := NewMavenExtractor().Extract(root)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	ls := metadata.LanguageSpecific
	assertSources(t, ls["generated_sources"].([]generatedSource), []generatedSource{
		{Module: "gen", Path: "src/main/java/generated", Plugin: "jaxb2-maven-plugin", Derivation: derivationConfigured},
	})
	if dirs, present := ls["generated_source_dirs"]; present {
		t.Errorf("generated_source_dirs = %v, want absent: the path sits inside extra's sources", dirs)
	}
}

// The flat list is safe only once every module's source directories are
// known, so the walk reaches modules however deeply aggregators nest. Here
// a source directory twelve levels down makes a shallow generator's path
// unsafe to publish.
func TestGeneratedSourceDirsCoverDeepModules(t *testing.T) {
	const depth = 12
	files := map[string]string{
		"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules>
        <module>gen</module>
        <module>l1</module>
    </modules>`),
		"gen/pom.xml": projectPOM(withPlugins(openAPIExecution("", "", `
                                <output>${project.basedir}</output>
                                <configOptions>
                                    <sourceFolder>src/gen/java</sourceFolder>
                                </configOptions>`))),
	}
	dir := ""
	for level := 1; level <= depth; level++ {
		dir = path.Join(dir, fmt.Sprintf("l%d", level))
		body := fmt.Sprintf("<packaging>pom</packaging><modules><module>l%d</module></modules>", level+1)
		if level == depth {
			body = "<build><sourceDirectory>src/gen/java</sourceDirectory></build>"
		}
		files[dir+"/pom.xml"] = projectPOM(body)
	}
	root := writeTree(t, files)

	metadata, err := NewMavenExtractor().Extract(root)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	ls := metadata.LanguageSpecific
	assertSources(t, ls["generated_sources"].([]generatedSource), []generatedSource{
		{Module: "gen", Path: "src/gen/java", Plugin: openAPI, Derivation: derivationConfigured},
	})
	if dirs, present := ls["generated_source_dirs"]; present {
		t.Errorf("generated_source_dirs = %v, want absent: src/gen/java holds sources in %s", dirs, dir)
	}
}

// Without a depth limit, the walk must still end when the module graph
// loops. A module that links back to the project resolves to a directory
// already visited.
func TestGeneratedSourcesWalkEndsOnModuleLoop(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pom.xml": projectPOM("<packaging>pom</packaging><modules><module>loop</module></modules>" +
			withPlugins(antlrBound)),
	})
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	assertSources(t, extractTree(t, root), []generatedSource{antlrDefault})
}

// A module path escaping the workspace must not be read, matching the
// guard every reactor traversal shares. The module still belongs to the
// reactor, so with its source roots unknown the flat list is withheld;
// generated_sources keeps the readable module's entry.
func TestGeneratedSourcesIgnoreModulesOutsideWorkspace(t *testing.T) {
	antlr := projectPOM(withPlugins(antlrBound))
	base := writeTree(t, map[string]string{
		"workspace/pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules>
        <module>inside</module>
        <module>../outside</module>
    </modules>`),
		"workspace/inside/pom.xml": antlr,
		"outside/pom.xml":          antlr,
	})
	workspace := filepath.Join(base, "workspace")
	t.Setenv("GITHUB_WORKSPACE", workspace)

	metadata, err := NewMavenExtractor().Extract(workspace)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	ls := metadata.LanguageSpecific
	assertSources(t, ls["generated_sources"].([]generatedSource), []generatedSource{
		{Module: "inside", Path: "target/generated-sources/antlr4",
			Plugin: "antlr4-maven-plugin", Derivation: derivationPluginDefault},
	})
	if dirs, present := ls["generated_source_dirs"]; present {
		t.Errorf("generated_source_dirs = %v, want absent: a module could not be read", dirs)
	}
}

// A module entry leading with ${basedir} and a separator names a directory
// inside the module. With no separator Maven joins the strings, naming a
// sibling of the module the action does not follow, so the entry counts
// unread and the flat list is withheld, even with a .child directory that
// a rewrite to a relative path would have read in its place.
func TestGeneratedSourcesReadBasedirModules(t *testing.T) {
	tests := []struct {
		name     string
		module   string
		wantDirs bool
	}{
		{name: "basedir and a separator", module: "${basedir}/child", wantDirs: true},
		{name: "basedir without a separator", module: "${basedir}child"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			child := projectPOM("")
			root := writeTree(t, map[string]string{
				"pom.xml": projectPOM(`
    <packaging>pom</packaging>
    <modules>
        <module>gen</module>
        <module>` + tc.module + `</module>
    </modules>`),
				"gen/pom.xml":    projectPOM(withPlugins(antlrBound)),
				"child/pom.xml":  child,
				".child/pom.xml": child,
			})

			metadata, err := NewMavenExtractor().Extract(root)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			ls := metadata.LanguageSpecific
			assertSources(t, ls["generated_sources"].([]generatedSource), []generatedSource{
				{Module: "gen", Path: "target/generated-sources/antlr4",
					Plugin: "antlr4-maven-plugin", Derivation: derivationPluginDefault},
			})
			if tc.wantDirs {
				assertStringSlice(t, ls, "generated_source_dirs", []string{"target/generated-sources/antlr4"})
			} else if dirs, present := ls["generated_source_dirs"]; present {
				t.Errorf("generated_source_dirs = %v, want absent: the module could not be read", dirs)
			}
		})
	}
}

func TestJavaFeatureRelease(t *testing.T) {
	tests := []struct {
		level   string
		release int
		ok      bool
	}{
		{"23", 23, true},
		{"17", 17, true},
		{"1.8", 8, true},
		{" 21 ", 21, true},
		{"", 0, false},
		{"${java.version}", 0, false},
	}
	for _, tc := range tests {
		release, ok := javaFeatureRelease(tc.level)
		if ok != tc.ok || (ok && release != tc.release) {
			t.Errorf("javaFeatureRelease(%q) = %d, %v; want %d, %v",
				tc.level, release, ok, tc.release, tc.ok)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		version, minimum string
		meets, ok        bool
	}{
		{"6.1.0", "6.1", true, true},
		{"7.12.0", "6.1", true, true},
		{"6.0.1", "6.1", false, true},
		{"5.4.0", "6.1", false, true},
		{"6.1-beta", "6.1", true, true},
		{"7", "6.1", true, true},
		{"", "6.1", false, false},
		{"${openapi.version}", "6.1", false, false},
		{"[6.0,7.0)", "6.1", false, false},
	}
	for _, tc := range tests {
		meets, ok := versionAtLeast(tc.version, tc.minimum)
		if meets != tc.meets || ok != tc.ok {
			t.Errorf("versionAtLeast(%q, %q) = %v, %v; want %v, %v",
				tc.version, tc.minimum, meets, ok, tc.meets, tc.ok)
		}
	}
}

// The JSON field names are the contract consumers parse.
func TestGeneratedSourceJSONFields(t *testing.T) {
	encoded, err := json.Marshal(rootSource("target/x", openAPI, derivationConfigured))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	want := `{"module":".","path":"target/x","plugin":"openapi-generator-maven-plugin","derivation":"configured"}`
	if string(encoded) != want {
		t.Errorf("json = %s, want %s", encoded, want)
	}
}
