// Package plugin ingests the output of an external scheduled plugin:
// a program the scheduled workflow runs in its own container,
// never code compiled into AurumCode. Its output is untrusted data. The
// package reads it through one adapter per format (adapter_sarif.go,
// adapter_vvah.go), refuses every finding whose path leaves the scanned
// tree (paths.go), and writes a SARIF 2.1.0 document for code scanning
// (sarif.go) whose text went through the secret filter.
//
// An inconclusive run (the plugin failed, its output is missing, malformed,
// of an unknown major version, or the plugin itself reports lost
// coverage) writes NO document: an empty SARIF uploaded under the same
// category would close every open alert. Ingest reports only counts.
package plugin
