@echo off
REM Run the AurumCode documentation generator in a container (Windows).

REM Credentials are not exported here: Compose reads .env from this directory
REM by itself. They are optional anyway - without them cmd/regenerate-docs
REM still generates documentation and only skips the AI welcome page.
if exist .env (
    echo .env found - Compose will substitute it
) else (
    echo No .env found - running without LLM credentials
    echo Optional: LLM_API_KEY, LLM_BASE_URL, LLM_MODEL ^(or OPENAI_API_KEY^)
)

echo.
echo Building image...
docker compose -f docker-compose.test.yml build
if %ERRORLEVEL% neq 0 (
    echo Docker build failed
    exit /b 1
)

echo.
echo Running documentation generator...
echo ---------------------------------------------
docker compose -f docker-compose.test.yml run --rm test-docs-pipeline
if %ERRORLEVEL% neq 0 (
    echo Generator failed
    exit /b 1
)
echo ---------------------------------------------

echo.
echo Generator finished. Markdown was written to the output directory.
echo.
echo Verify with:
echo   git status
echo   dir .aurumcode
