@echo off
REM Run AurumCode Documentation Pipeline in Docker (Windows)

echo 🚀 Running AurumCode Documentation Pipeline in Docker
echo.

REM Check if .env exists
if not exist .env (
    echo ❌ .env file not found!
    echo Please create .env with:
    echo   PROVEDOR_API_KEY=your_key
    echo   PROVEDOR_BASE_URL=your_url
    exit /b 1
)

echo ✓ .env file found
echo.

REM Build Docker image
echo 📦 Building Docker image...
docker-compose -f docker-compose.test.yml build

if %ERRORLEVEL% neq 0 (
    echo ❌ Docker build failed
    exit /b 1
)

echo.
echo 🏃 Running Documentation Pipeline...
echo ─────────────────────────────────────────
docker-compose -f docker-compose.test.yml run --rm test-docs-pipeline
echo ─────────────────────────────────────────

echo.
echo ✅ Pipeline completed!
echo.
echo 📊 Check generated files:
echo   - CHANGELOG.md
echo   - README.md (updated)
echo.
echo Verify with:
echo   git status
echo   git diff README.md
