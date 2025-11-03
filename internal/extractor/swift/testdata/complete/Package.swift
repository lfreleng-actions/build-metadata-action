// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "ExampleSwiftPackage",
    platforms: [
        .macOS(.v13),
        .iOS(.v16),
        .tvOS(.v16),
        .watchOS(.v9)
    ],
    products: [
        .library(
            name: "ExampleSwiftPackage",
            targets: ["ExampleSwiftPackage"]),
        .executable(
            name: "example-cli",
            targets: ["ExampleCLI"])
    ],
    dependencies: [
        .package(url: "https://github.com/apple/swift-argument-parser.git", from: "1.2.0"),
        .package(url: "https://github.com/apple/swift-log.git", from: "1.5.0")
    ],
    targets: [
        .target(
            name: "ExampleSwiftPackage",
            dependencies: [
                .product(name: "Logging", package: "swift-log")
            ]),
        .executableTarget(
            name: "ExampleCLI",
            dependencies: [
                "ExampleSwiftPackage",
                .product(name: "ArgumentParser", package: "swift-argument-parser")
            ]),
        .testTarget(
            name: "ExampleSwiftPackageTests",
            dependencies: ["ExampleSwiftPackage"]),
    ],
    cLanguageStandard: .c11,
    cxxLanguageStandard: .cxx17
)
