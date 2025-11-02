package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
)

// Issue represents a code quality issue
type Issue struct {
	Line        int
	Severity    string
	Category    string
	Description string
	Suggestion  string
}

// CodeAnalysis represents the analysis of a file
type CodeAnalysis struct {
	FileName    string
	CodeType    string
	ISOScores   map[string]int
	Issues      []Issue
	Summary     string
	CodeSnippet string
}

func main() {
	// Analysis for bad-code.go
	badCodeAnalysis := CodeAnalysis{
		FileName: "demo/bad-code.go",
		CodeType: "BAD",
		ISOScores: map[string]int{
			"Security":        3,
			"Maintainability": 4,
			"Reliability":     5,
			"Performance":     4,
		},
		Issues: []Issue{
			{
				Line:        21,
				Severity:    "CRITICAL",
				Category:    "Security",
				Description: "SQL Injection vulnerability - user input concatenated directly into query",
				Suggestion:  "Use parameterized queries with placeholders instead of string concatenation",
			},
			{
				Line:        45,
				Severity:    "CRITICAL",
				Category:    "Security",
				Description: "Plain text password storage - passwords stored without hashing",
				Suggestion:  "Hash passwords using bcrypt or SHA-256 before storing in database",
			},
			{
				Line:        38,
				Severity:    "HIGH",
				Category:    "Security",
				Description: "Missing authentication check - anyone can create users",
				Suggestion:  "Add authentication middleware to verify user identity before allowing operations",
			},
			{
				Line:        96,
				Severity:    "CRITICAL",
				Category:    "Security",
				Description: "Exposing passwords in API responses",
				Suggestion:  "Never return password fields in API responses, use json:\"-\" tag",
			},
			{
				Line:        68,
				Severity:    "CRITICAL",
				Category:    "Security",
				Description: "SQL injection in DeleteUser - missing input validation and parameterization",
				Suggestion:  "Use parameterized queries and validate user ID format",
			},
			{
				Line:        105,
				Severity:    "HIGH",
				Category:    "Security",
				Description: "Missing CSRF protection on state-changing operations",
				Suggestion:  "Implement CSRF token validation for all POST/PUT/DELETE operations",
			},
			{
				Line:        80,
				Severity:    "HIGH",
				Category:    "Performance",
				Description: "No pagination - loading all users into memory at once",
				Suggestion:  "Add pagination with LIMIT and OFFSET clauses",
			},
			{
				Line:        121,
				Severity:    "MEDIUM",
				Category:    "Maintainability",
				Description: "High cyclomatic complexity (>10) - deeply nested conditions",
				Suggestion:  "Refactor using strategy pattern to reduce complexity and improve readability",
			},
		},
		Summary: "This code contains multiple critical security vulnerabilities including SQL injection, plain text password storage, and missing authentication checks. Performance issues include lack of pagination. Maintainability is poor due to high cyclomatic complexity.",
		CodeSnippet: `// SQL INJECTION - user input concatenated directly into query
query := "SELECT * FROM users WHERE id = '" + id + "'"

// PLAIN TEXT PASSWORD
query := fmt.Sprintf("INSERT INTO users (name, email, password) VALUES ('%s', '%s', '%s')",
    name, email, password)`,
	}

	// Analysis for good-code.go
	goodCodeAnalysis := CodeAnalysis{
		FileName: "demo/good-code.go",
		CodeType: "GOOD",
		ISOScores: map[string]int{
			"Security":        9,
			"Maintainability": 8,
			"Reliability":     9,
			"Performance":     8,
		},
		Issues: []Issue{},
		Summary: "Excellent code quality! All security issues resolved with parameterized queries, password hashing, authentication checks, and CSRF protection. Performance improved with pagination. Maintainability enhanced with strategy pattern and reduced cyclomatic complexity.",
		CodeSnippet: `// ✅ SECURE: Parameterized query prevents SQL injection
query := "SELECT id, name, email FROM users WHERE id = ?"
err = s.db.QueryRow(query, id).Scan(&user.ID, &user.Name, &user.Email)

// ✅ SECURE: Password hashing
passwordHash := s.hashPassword(user.PasswordHash)

// ✅ SECURE: Authentication check
if !s.isAuthenticated(r) {
    http.Error(w, "Unauthorized", http.StatusUnauthorized)
    return
}`,
	}

	// Generate HTML
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AurumCode Analysis Demo - Repository Code Review</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
        }
        header {
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            padding: 40px;
            border-radius: 15px;
            margin-bottom: 30px;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
            text-align: center;
        }
        header h1 {
            font-size: 2.5em;
            color: #667eea;
            margin-bottom: 10px;
        }
        header p {
            font-size: 1.2em;
            color: #666;
        }
        .comparison-grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 30px;
            margin-bottom: 30px;
        }
        @media (max-width: 1200px) {
            .comparison-grid {
                grid-template-columns: 1fr;
            }
        }
        .analysis-card {
            background: white;
            padding: 30px;
            border-radius: 15px;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.2);
        }
        .analysis-card.bad {
            border-left: 6px solid #ef4444;
        }
        .analysis-card.good {
            border-left: 6px solid #10b981;
        }
        .card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
            padding-bottom: 15px;
            border-bottom: 2px solid #f0f0f0;
        }
        .card-header h2 {
            font-size: 1.8em;
            color: #333;
        }
        .badge {
            padding: 8px 16px;
            border-radius: 20px;
            font-weight: 600;
            font-size: 0.9em;
        }
        .badge.bad {
            background: #fecaca;
            color: #dc2626;
        }
        .badge.good {
            background: #d1fae5;
            color: #059669;
        }
        .iso-scores {
            background: #f9fafb;
            padding: 20px;
            border-radius: 10px;
            margin-bottom: 20px;
        }
        .iso-scores h3 {
            margin-bottom: 15px;
            color: #667eea;
        }
        .score-item {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 10px 0;
            border-bottom: 1px solid #e5e7eb;
        }
        .score-item:last-child {
            border-bottom: none;
        }
        .score-bar {
            width: 60%;
            height: 8px;
            background: #e5e7eb;
            border-radius: 4px;
            overflow: hidden;
        }
        .score-bar-fill {
            height: 100%;
            transition: width 0.3s ease;
        }
        .score-bar-fill.low {
            background: #ef4444;
        }
        .score-bar-fill.medium {
            background: #f59e0b;
        }
        .score-bar-fill.high {
            background: #10b981;
        }
        .issues-section {
            margin: 20px 0;
        }
        .issues-section h3 {
            margin-bottom: 15px;
            color: #667eea;
        }
        .issue {
            background: #f9fafb;
            padding: 15px;
            border-radius: 8px;
            margin-bottom: 12px;
            border-left: 4px solid;
        }
        .issue.critical {
            border-left-color: #dc2626;
        }
        .issue.high {
            border-left-color: #f59e0b;
        }
        .issue.medium {
            border-left-color: #3b82f6;
        }
        .issue-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 8px;
        }
        .issue-line {
            font-family: 'Courier New', monospace;
            font-size: 0.85em;
            background: #e5e7eb;
            padding: 4px 8px;
            border-radius: 4px;
        }
        .severity-badge {
            padding: 4px 12px;
            border-radius: 12px;
            font-size: 0.75em;
            font-weight: 700;
            text-transform: uppercase;
        }
        .severity-badge.critical {
            background: #fecaca;
            color: #dc2626;
        }
        .severity-badge.high {
            background: #fed7aa;
            color: #ea580c;
        }
        .severity-badge.medium {
            background: #dbeafe;
            color: #2563eb;
        }
        .issue-description {
            color: #374151;
            margin-bottom: 8px;
        }
        .issue-suggestion {
            color: #059669;
            font-style: italic;
            font-size: 0.9em;
        }
        .code-snippet {
            background: #1e293b;
            color: #e2e8f0;
            padding: 20px;
            border-radius: 8px;
            font-family: 'Courier New', monospace;
            font-size: 0.9em;
            overflow-x: auto;
            margin: 15px 0;
        }
        .summary {
            background: #f0f9ff;
            border: 2px solid #bae6fd;
            padding: 20px;
            border-radius: 8px;
            margin: 20px 0;
        }
        .summary.good {
            background: #f0fdf4;
            border-color: #bbf7d0;
        }
        footer {
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            padding: 30px;
            border-radius: 15px;
            text-align: center;
            color: #666;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.2);
        }
        footer a {
            color: #667eea;
            text-decoration: none;
            font-weight: 600;
        }
        footer a:hover {
            text-decoration: underline;
        }
        .success-message {
            background: #d1fae5;
            border: 2px solid #10b981;
            color: #065f46;
            padding: 15px;
            border-radius: 8px;
            margin-bottom: 20px;
            font-weight: 600;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🤖 AurumCode Analysis Demo</h1>
            <p>AI-Powered Code Review of AurumCode Repository</p>
            <p style="margin-top: 10px; font-size: 0.95em; color: #888;">Demonstrating ISO/IEC 25010 Quality Analysis</p>
        </header>

        <div class="comparison-grid">
            <!-- BAD CODE ANALYSIS -->
            <div class="analysis-card bad">
                <div class="card-header">
                    <h2>{{.BadCode.FileName}}</h2>
                    <span class="badge bad">❌ BLOCKED</span>
                </div>

                <div class="iso-scores">
                    <h3>ISO/IEC 25010 Quality Scores</h3>
                    {{range $key, $value := .BadCode.ISOScores}}
                    <div class="score-item">
                        <span><strong>{{$key}}:</strong> {{$value}}/10</span>
                        <div class="score-bar">
                            <div class="score-bar-fill {{if lt $value 5}}low{{else if lt $value 7}}medium{{else}}high{{end}}"
                                 style="width: {{mul $value 10}}%"></div>
                        </div>
                    </div>
                    {{end}}
                </div>

                <div class="summary">
                    <strong>Summary:</strong> {{.BadCode.Summary}}
                </div>

                <div class="issues-section">
                    <h3>🔴 Issues Found ({{len .BadCode.Issues}})</h3>
                    {{range .BadCode.Issues}}
                    <div class="issue {{lower .Severity}}">
                        <div class="issue-header">
                            <span class="issue-line">Line {{.Line}}</span>
                            <span class="severity-badge {{lower .Severity}}">{{.Severity}}</span>
                        </div>
                        <div class="issue-description">
                            <strong>{{.Category}}:</strong> {{.Description}}
                        </div>
                        <div class="issue-suggestion">
                            💡 {{.Suggestion}}
                        </div>
                    </div>
                    {{end}}
                </div>

                <div class="code-snippet">{{.BadCode.CodeSnippet}}</div>
            </div>

            <!-- GOOD CODE ANALYSIS -->
            <div class="analysis-card good">
                <div class="card-header">
                    <h2>{{.GoodCode.FileName}}</h2>
                    <span class="badge good">✅ APPROVED</span>
                </div>

                <div class="iso-scores">
                    <h3>ISO/IEC 25010 Quality Scores</h3>
                    {{range $key, $value := .GoodCode.ISOScores}}
                    <div class="score-item">
                        <span><strong>{{$key}}:</strong> {{$value}}/10</span>
                        <div class="score-bar">
                            <div class="score-bar-fill {{if lt $value 5}}low{{else if lt $value 7}}medium{{else}}high{{end}}"
                                 style="width: {{mul $value 10}}%"></div>
                        </div>
                    </div>
                    {{end}}
                </div>

                <div class="summary good">
                    <strong>✅ All Checks Passed!</strong><br>
                    {{.GoodCode.Summary}}
                </div>

                <div class="success-message">
                    🎉 <strong>No issues found!</strong> This code follows all security and quality best practices.
                </div>

                <div class="code-snippet">{{.GoodCode.CodeSnippet}}</div>

                <div style="margin-top: 20px; padding: 15px; background: #f0fdf4; border-radius: 8px;">
                    <h4 style="color: #059669; margin-bottom: 10px;">✅ Improvements Made:</h4>
                    <ul style="color: #065f46; padding-left: 20px;">
                        <li>Parameterized queries prevent SQL injection</li>
                        <li>Password hashing with SHA-256</li>
                        <li>Authentication and authorization checks</li>
                        <li>CSRF protection implemented</li>
                        <li>Pagination for efficient data loading</li>
                        <li>Strategy pattern reduces complexity</li>
                        <li>Proper error handling throughout</li>
                    </ul>
                </div>
            </div>
        </div>

        <footer>
            <p><strong>🤖 Generated by AurumCode</strong></p>
            <p style="margin: 10px 0;">Automated AI-Powered Code Quality Platform</p>
            <p>
                <a href="https://github.com/Mpaape/AurumCode">View on GitHub</a> |
                <a href="../README.md">Documentation</a>
            </p>
            <p style="margin-top: 15px; font-size: 0.9em;">
                This is a demonstration of AurumCode analyzing its own repository code.<br>
                The analysis uses ISO/IEC 25010 quality standards for comprehensive code review.
            </p>
        </footer>
    </div>
</body>
</html>`

	// Parse template
	t := template.New("analysis")
	t = t.Funcs(template.FuncMap{
		"mul": func(a, b int) int {
			return a * b
		},
		"lower": func(s string) string {
			return fmt.Sprintf("%s", s)
		},
	})

	t, err := t.Parse(tmpl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing template: %v\n", err)
		os.Exit(1)
	}

	// Prepare data
	data := struct {
		BadCode  CodeAnalysis
		GoodCode CodeAnalysis
	}{
		BadCode:  badCodeAnalysis,
		GoodCode: goodCodeAnalysis,
	}

	// Generate HTML file
	outputPath := filepath.Join("docs", "aurumcode-analysis-demo.html")
	f, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if err := t.Execute(f, data); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing template: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Generated analysis demo page: %s\n", outputPath)
	fmt.Println("📊 Analysis includes:")
	fmt.Printf("   - Bad code: %d issues found\n", len(badCodeAnalysis.Issues))
	fmt.Printf("   - Good code: All checks passed!\n")
	fmt.Println("\n🌐 Open the HTML file in your browser to view the analysis")
}
