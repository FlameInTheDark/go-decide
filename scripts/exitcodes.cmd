@echo off
REM Exit code matrix for the decide CLI.
REM Run from the repository root after: go build -o decide.exe ./cmd/decide
setlocal EnableExtensions

set BIN=%~1
if "%BIN%"=="" set BIN=.\decide.exe

call :check 0 version version
call :check 2 "x" -o "bad"
call :check 3 "x" --provider openrouter
call :check 4 "x" -m nosuchmodel
call :check 2
call :check 4 "x" --provider nope

endlocal
exit /b 0

:check
set EXPECTED=%~1
shift
%BIN% %1 %2 %3 %4 %5 %6 %7 %8 %9 > check.out 2> check.err
set ACTUAL=%errorlevel%
if "%ACTUAL%"=="%EXPECTED%" (
    echo PASS  args=[%1 %2 %3 %4 %5 %6 %7 %8 %9]  exit=%ACTUAL%
) else (
    echo FAIL  args=[%1 %2 %3 %4 %5 %6 %7 %8 %9]  exit=%ACTUAL% expected=%EXPECTED%
    type check.out
    type check.err
)
goto :eof