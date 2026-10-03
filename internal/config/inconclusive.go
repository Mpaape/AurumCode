package config

// InconclusiveMode resolves gate.inconclusive against the effective
// configuration. When the key is absent, the default is block whenever the
// configuration declares a gate or enables a scanner (SAST, Dependency-Track,
// analysis data): a tool that was asked to verify and could not must never
// approve by omission. Avisar (warn) only happens when "warn" is written.
// With neither a gate nor a scanner, nothing is gated and the mode is "".
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
	return c.QualityGates.Sast.IsEnabled() || c.QualityGates.SsorDtrack.Declared() || c.AnalysisData.Declared()
}
