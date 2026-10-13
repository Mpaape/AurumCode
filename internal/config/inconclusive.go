package config

// InconclusiveMode resolves gate.inconclusive against the effective
// configuration. When the key is absent, the default is block whenever the
// configuration declares a gate or switches on any deterministic check
// (scannerEnabled: an enabled scanner engine, Dependency-Track, analysis
// data or the dependencies gate): a tool that was asked to verify and could
// not must never approve by omission. Avisar (warn) only happens when
// "warn" is written. With neither a gate nor a check, nothing is gated and
// the mode is "".
func (c *Config) InconclusiveMode() (string, error) {
	if c == nil {
		return "", nil
	}
	return c.Gate.resolveInconclusive(c.defaultInconclusiveMode())
}

func (c *Config) defaultInconclusiveMode() string {
	if c.Gate.Declared() || c.scannerEnabled() {
		return InconclusiveBlock
	}
	return ""
}

// scannerEnabled reports whether any deterministic scanner section of the
// effective configuration is switched on.
func (c *Config) scannerEnabled() bool {
	return len(c.QualityGates.EnabledScanners()) > 0 || c.QualityGates.SsorDtrack.Declared() || c.AnalysisData.Declared() || c.Dependencies.Declared()
}
