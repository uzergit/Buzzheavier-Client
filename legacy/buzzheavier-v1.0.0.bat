@echo off
setlocal EnableExtensions EnableDelayedExpansion

rem buzzheavier uploader - made by uzer (github.com/uzergit)
rem uses curl + powershell, both come with windows 10/11

set "APP_NAME=BuzzHeavier Uploader"
set "SETTINGS_FILE=%~dp0buzz_settings.ini"
set "TMP_RESPONSE=%~dp0buzz_response.tmp"
set "TMP_STATUS=%~dp0buzz_status.tmp"
set "LOG_FILE=%~dp0buzz_uploads.log"
set "DEFAULT_MODE=anonymous"
set "LAST_MODE="
set "ACCOUNT_ID="
set "PARENT_ID="
set "LOCATION_ID="
set "NOTE_TEXT="

rem first run: make a settings file, then read whatever is in it
if not exist "%SETTINGS_FILE%" (
    >"%SETTINGS_FILE%" (
        echo account=
        echo default=anonymous
        echo last_mode=
    )
)

for /f "usebackq tokens=1* delims==" %%A in ("%SETTINGS_FILE%") do (
    if /i "%%A"=="account" set "ACCOUNT_ID=%%B"
    if /i "%%A"=="default" set "DEFAULT_MODE=%%B"
    if /i "%%A"=="last_mode" set "LAST_MODE=%%B"
)

rem anything weird in the ini just falls back to anonymous
if /i not "%DEFAULT_MODE%"=="anonymous" if /i not "%DEFAULT_MODE%"=="account" set "DEFAULT_MODE=anonymous"
if /i "%LAST_MODE%"=="account" set "LAST_MODE=token"
if not defined LAST_MODE (
    if /i "%DEFAULT_MODE%"=="account" (
        set "LAST_MODE=token"
    ) else (
        set "LAST_MODE=%DEFAULT_MODE%"
    )
)

rem main menu
:MENU
cls
echo ================================================
echo  Buzzheavier client Made by uzergit on Github
echo  https://github.com/uzergit/Buzzheavier-Client
echo  Version: 1 - 5th October 2025
echo ================================================
echo.
echo  1. Anonymous upload
echo  2. Upload with token
echo  3. Settings
echo  4. API tools
echo  5. Quit
echo.
echo  Quick upload: paste a full file path here and press Enter.
echo ----------------------------------------------
if defined ACCOUNT_ID (
    echo  Current Account ID: [set]
) else (
    echo  Current Account ID: [not set]
)
if not defined LAST_MODE set "LAST_MODE=%DEFAULT_MODE%"
echo  Default upload mode: %DEFAULT_MODE%
echo  Last upload mode: %LAST_MODE%
echo ----------------------------------------------
set "USER_INPUT="
set /p "USER_INPUT=Select option (1-5) or paste a file path: "

if "%USER_INPUT%"=="1" goto ANON_UPLOAD_PROMPT
if "%USER_INPUT%"=="2" goto TOKEN_UPLOAD_PROMPT
if "%USER_INPUT%"=="3" goto SETTINGS
if "%USER_INPUT%"=="4" goto API_TOOLS
if "%USER_INPUT%"=="5" goto QUIT

rem not a menu number so it is probably a path someone pasted or dragged in
set "FILEPATH_RAW=%USER_INPUT%"
if not defined FILEPATH_RAW goto MENU
call :QUICK_UPLOAD "%FILEPATH_RAW%"
goto MENU

rem quick upload, reuses whatever mode was used last time
:QUICK_UPLOAD
set "INPUT_PATH=%~1"
for %%I in ("%INPUT_PATH%") do (
    set "FILEPATH=%%~fI"
    set "FILENAME=%%~nxI"
)
if not exist "%FILEPATH%" (
    echo.
    echo Error: File not found: %INPUT_PATH%
    call :PAUSE
    goto :eof
)
if /i "%LAST_MODE%"=="token" (
    if defined ACCOUNT_ID (
        call :DO_UPLOAD "token" "%FILEPATH%" "%FILENAME%"
    ) else (
        echo.
        echo Last mode was token but no Account ID is set. Falling back to anonymous.
        call :DO_UPLOAD "anonymous" "%FILEPATH%" "%FILENAME%"
    )
) else (
    call :DO_UPLOAD "anonymous" "%FILEPATH%" "%FILENAME%"
)

goto :eof

rem anon upload
:ANON_UPLOAD_PROMPT
cls
echo Anonymous upload
echo -----------------
set "FP="
set /p "FP=Enter file path to upload: "
if not defined FP goto MENU
call :CHECK_AND_NORMALIZE "%FP%"
if errorlevel 1 (
    call :PAUSE
    goto MENU
)
call :DO_UPLOAD "anonymous" "%FILEPATH%" "%FILENAME%"
goto MENU

rem upload with the account token
:TOKEN_UPLOAD_PROMPT
cls
echo Upload with token
echo -----------------
if not defined ACCOUNT_ID (
    echo Error: No Account ID set. Go to Settings to configure your token.
    call :PAUSE
    goto MENU
)
set "FP="
set /p "FP=Enter file path to upload: "
if not defined FP goto MENU
call :CHECK_AND_NORMALIZE "%FP%"
if errorlevel 1 (
    call :PAUSE
    goto MENU
)
call :DO_UPLOAD "token" "%FILEPATH%" "%FILENAME%"
goto MENU

rem settings
:SETTINGS
cls
echo Settings
echo --------
echo  1. Set / change Account ID
echo  2. Set default upload mode (anonymous or account token)
echo  3. Clear Account ID
echo  4. Show current settings
echo  5. Reset to defaults
echo  6. Set last upload mode
echo  7. Back to main menu
echo.
set "SOPT="
set /p "SOPT=Choose an option (1-7): "
if "%SOPT%"=="1" goto SET_ACCOUNT
if "%SOPT%"=="2" goto SET_DEFAULT
if "%SOPT%"=="3" goto CLEAR_ACCOUNT
if "%SOPT%"=="4" goto SHOW_SETTINGS
if "%SOPT%"=="5" goto RESET_IN_SETTINGS
if "%SOPT%"=="6" goto SET_LAST_MODE
if "%SOPT%"=="7" goto MENU
goto MENU

:SET_ACCOUNT
cls
set "NEWACC="
set /p "NEWACC=Enter new Account ID (token): "
if not defined NEWACC goto SETTINGS
set "ACCOUNT_ID=%NEWACC%"
call :SAVE_SETTINGS
echo Saved.
call :PAUSE
goto SETTINGS

:SET_DEFAULT
cls
echo Current default: %DEFAULT_MODE%
echo  1. anonymous
echo  2. account
set "DM="
set /p "DM=Choose default (1-2): "
if "%DM%"=="1" set "DEFAULT_MODE=anonymous" & call :SAVE_SETTINGS & goto SETTINGS
if "%DM%"=="2" set "DEFAULT_MODE=account" & call :SAVE_SETTINGS & goto SETTINGS
goto SETTINGS

:CLEAR_ACCOUNT
cls
set "ACCOUNT_ID="
call :SAVE_SETTINGS
echo Account ID cleared.
call :PAUSE
goto SETTINGS

:SHOW_SETTINGS
cls
echo Current settings:
echo -----------------
echo Account ID: %ACCOUNT_ID%
echo Default mode: %DEFAULT_MODE%
echo Last upload mode: %LAST_MODE%
echo Settings file: %SETTINGS_FILE%
echo Upload log: %LOG_FILE%
echo.
call :PAUSE
goto SETTINGS

:SET_LAST_MODE
cls
echo Last upload mode: %LAST_MODE%
echo  1. anonymous
echo  2. token
set "LM="
set /p "LM=Choose last mode (1-2): "
if "%LM%"=="1" set "LAST_MODE=anonymous" & call :SAVE_SETTINGS & goto SETTINGS
if "%LM%"=="2" set "LAST_MODE=token" & call :SAVE_SETTINGS & goto SETTINGS
goto SETTINGS

rem reset - wipes this folder except the bat itself
:RESET_IN_SETTINGS
cls
echo WARNING: This will delete ALL files in this folder and reset settings to defaults.
echo It will remove settings, logs, temp files, and other files except this batch file.
echo.
set "BATNAME=%~nx0"
echo This batch file that will be kept: %BATNAME%
echo.
set /p "CONF=Type YES to confirm and delete files, or anything else to cancel: "
if /i not "%CONF%"=="YES" (
    echo Cancelled.
    call :PAUSE
    goto SETTINGS
)
echo Deleting files...
for %%F in (*) do (
    if /i not "%%F"=="%BATNAME%" del /q "%%F" 2>nul
)
rem fresh settings file
set "ACCOUNT_ID="
set "DEFAULT_MODE=anonymous"
set "LAST_MODE="
call :SAVE_SETTINGS
echo Reset complete. Only %BATNAME% remains.
call :PAUSE
goto SETTINGS

rem this is where the upload actually happens
:DO_UPLOAD
set "MODE=%~1"
set "FILEPATH=%~2"
set "FILENAME_RAW=%~3"
set "PARENT_ID="
set "LOCATION_ID="
set "NOTE_TEXT="
set "NOTE_B64="

rem filenames with spaces etc need encoding, let powershell deal with it
set "FILENAME_URL="
for /f "usebackq delims=" %%E in (`powershell -NoProfile -Command "[System.Uri]::EscapeDataString('%FILENAME_RAW%')"`) do set "FILENAME_URL=%%E"
if not defined FILENAME_URL set "FILENAME_URL=%FILENAME_RAW%"

set "UPLOAD_URL=https://w.buzzheavier.com/%FILENAME_URL%"

if /i "%MODE%"=="token" (
    set /p "PARENT_ID=Parent directory ID (optional, blank for root): "
)
set /p "LOCATION_ID=Location ID (optional, blank for default): "
set /p "NOTE_TEXT=Note (optional, blank for none): "

if defined NOTE_TEXT (
    for /f "usebackq delims=" %%N in (`powershell -NoProfile -Command "[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('%NOTE_TEXT%'))"`) do set "NOTE_B64=%%N"
)

if defined PARENT_ID (
    set "UPLOAD_URL=https://w.buzzheavier.com/%PARENT_ID%/%FILENAME_URL%"
)

if defined LOCATION_ID (
    if defined NOTE_B64 (
        set "UPLOAD_URL=%UPLOAD_URL%?locationId=%LOCATION_ID%^&note=%NOTE_B64%"
    ) else (
        set "UPLOAD_URL=%UPLOAD_URL%?locationId=%LOCATION_ID%"
    )
) else if defined NOTE_B64 (
    set "UPLOAD_URL=%UPLOAD_URL%?note=%NOTE_B64%"
)

cls
echo Uploading: %FILEPATH%
echo Mode: %MODE%
echo ----------------------------------------

del /q "%TMP_RESPONSE%" 2>nul
del /q "%TMP_STATUS%" 2>nul

if /i "%MODE%"=="token" (
    call :CURL_UPLOAD "Authorization: Bearer %ACCOUNT_ID%"
    if errorlevel 1 goto CURL_FAILED
) else (
    call :CURL_UPLOAD ""
    if errorlevel 1 goto CURL_FAILED
)

goto AFTER_CURL

:CURL_FAILED
echo.
echo Upload failed. curl error code: %errorlevel%
if exist "%TMP_RESPONSE%" (
    echo Raw response:
    type "%TMP_RESPONSE%"
)
call :PAUSE
set "LAST_MODE=%MODE%"
call :SAVE_SETTINGS
goto MENU

:AFTER_CURL

rem grab the file id out of the json and turn it into a link
set "FINAL_LINK="
for /f "usebackq delims=" %%J in (`powershell -NoProfile -Command ^
    "try { $json = Get-Content -Raw '%TMP_RESPONSE%' | ConvertFrom-Json; if ($json.data.id) { Write-Output ('https://buzzheavier.com/' + $json.data.id) } elseif ($json.id) { Write-Output ('https://buzzheavier.com/' + $json.id) } } catch { }"`) do set "FINAL_LINK=%%J"

if defined FINAL_LINK (
    echo.
    echo Upload complete.
    echo Link: %FINAL_LINK%
    echo %FINAL_LINK% | clip
    echo.
    cmd /c echo Copied to clipboard!
    rem log it
    for /f "usebackq delims=" %%T in (`powershell -NoProfile -Command "Write-Output ((Get-Date).ToString('yyyy-MM-dd HH:mm:ss'))"`) do set "TIMESTAMP=%%T"
    if not defined TIMESTAMP set "TIMESTAMP=Unknown-Time"
    >>"%LOG_FILE%" echo [%TIMESTAMP%] - %FILENAME_RAW% - %FINAL_LINK%
    echo.
    echo Entry saved in log: %LOG_FILE%
    set "LAST_MODE=%MODE%"
    call :SAVE_SETTINGS
) else (
    echo.
    echo Upload complete, but could not parse link.
    echo Raw server response:
    type "%TMP_RESPONSE%"
    echo.
    echo Note: the link above may still be valid; check the raw response for "data.id".
    set "LAST_MODE=%MODE%"
    call :SAVE_SETTINGS
)

echo.
echo.
echo.
echo  1. Return to main menu
echo  2. Quit
:AFTER_UPLOAD_CHOICE
set "CHOICE="
set /p "CHOICE=Select an option: "
if "%CHOICE%"=="1" goto MENU
if "%CHOICE%"=="2" goto QUIT
echo Invalid choice, please enter 1 or 2.
goto AFTER_UPLOAD_CHOICE

rem api stuff (needs the token)
:API_TOOLS
cls
echo API Tools (token required)
echo --------------------------
echo  1. Get account info
echo  2. Get locations
echo  3. Get root directory
echo  4. Get directory by ID
echo  5. Create directory
echo  6. Rename item (file or directory)
echo  7. Move item (file or directory)
echo  8. Add note to file
echo  9. Delete directory
echo 10. Back to main menu
echo.
if not defined ACCOUNT_ID (
    echo Error: No Account ID set. Go to Settings to configure your token.
    call :PAUSE
    goto MENU
)
set "APIOPT="
set /p "APIOPT=Choose an option (1-10): "
if "%APIOPT%"=="1" goto API_ACCOUNT
if "%APIOPT%"=="2" goto API_LOCATIONS
if "%APIOPT%"=="3" goto API_FS_ROOT
if "%APIOPT%"=="4" goto API_FS_DIR
if "%APIOPT%"=="5" goto API_FS_CREATE
if "%APIOPT%"=="6" goto API_FS_RENAME
if "%APIOPT%"=="7" goto API_FS_MOVE
if "%APIOPT%"=="8" goto API_FS_NOTE
if "%APIOPT%"=="9" goto API_FS_DELETE
goto MENU

:API_ACCOUNT
cls
echo Fetching account info...
curl -s -H "Authorization: Bearer %ACCOUNT_ID%" "https://buzzheavier.com/api/account"
echo.
call :PAUSE
goto API_TOOLS

:API_LOCATIONS
cls
echo Fetching locations...
curl -s -H "Authorization: Bearer %ACCOUNT_ID%" "https://buzzheavier.com/api/locations"
echo.
call :PAUSE
goto API_TOOLS

:API_FS_ROOT
cls
echo Fetching root directory...
curl -s -H "Authorization: Bearer %ACCOUNT_ID%" "https://buzzheavier.com/api/fs"
echo.
call :PAUSE
goto API_TOOLS

:API_FS_DIR
cls
set "DIR_ID="
set /p "DIR_ID=Enter directory ID: "
if not defined DIR_ID goto API_TOOLS
curl -s -H "Authorization: Bearer %ACCOUNT_ID%" "https://buzzheavier.com/api/fs/%DIR_ID%"
echo.
call :PAUSE
goto API_TOOLS

:API_FS_CREATE
cls
set "PARENT_DIR="
set "NEW_NAME="
set /p "PARENT_DIR=Enter parent directory ID: "
if not defined PARENT_DIR goto API_TOOLS
set /p "NEW_NAME=Enter new directory name: "
if not defined NEW_NAME goto API_TOOLS
curl -s -X POST -H "Authorization: Bearer %ACCOUNT_ID%" -H "Content-Type: application/json" -d "{\"name\":\"%NEW_NAME%\"}" "https://buzzheavier.com/api/fs/%PARENT_DIR%"
echo.
call :PAUSE
goto API_TOOLS

:API_FS_RENAME
cls
set "ITEM_ID="
set "NEW_NAME="
set /p "ITEM_ID=Enter file or directory ID: "
if not defined ITEM_ID goto API_TOOLS
set /p "NEW_NAME=Enter new name: "
if not defined NEW_NAME goto API_TOOLS
curl -s -X PATCH -H "Authorization: Bearer %ACCOUNT_ID%" -H "Content-Type: application/json" -d "{\"name\":\"%NEW_NAME%\"}" "https://buzzheavier.com/api/fs/%ITEM_ID%"
echo.
call :PAUSE
goto API_TOOLS

:API_FS_MOVE
cls
set "ITEM_ID="
set "NEW_PARENT="
set /p "ITEM_ID=Enter file or directory ID: "
if not defined ITEM_ID goto API_TOOLS
set /p "NEW_PARENT=Enter new parent directory ID: "
if not defined NEW_PARENT goto API_TOOLS
curl -s -X PUT -H "Authorization: Bearer %ACCOUNT_ID%" -H "Content-Type: application/json" -d "{\"parentId\":\"%NEW_PARENT%\"}" "https://buzzheavier.com/api/fs/%ITEM_ID%"
echo.
call :PAUSE
goto API_TOOLS

:API_FS_NOTE
cls
set "FILE_ID="
set "NOTE_TEXT="
set /p "FILE_ID=Enter file ID: "
if not defined FILE_ID goto API_TOOLS
set /p "NOTE_TEXT=Enter note text: "
if not defined NOTE_TEXT goto API_TOOLS
curl -s -X PUT -H "Authorization: Bearer %ACCOUNT_ID%" -H "Content-Type: application/json" -d "{\"note\":\"%NOTE_TEXT%\"}" "https://buzzheavier.com/api/fs/%FILE_ID%"
echo.
call :PAUSE
goto API_TOOLS

:API_FS_DELETE
cls
set "DIR_ID="
set /p "DIR_ID=Enter directory ID to delete: "
if not defined DIR_ID goto API_TOOLS
curl -s -X DELETE -H "Authorization: Bearer %ACCOUNT_ID%" "https://buzzheavier.com/api/fs/%DIR_ID%"
echo.
call :PAUSE
goto API_TOOLS

rem helpers
:CURL_UPLOAD
set "AUTH_HEADER=%~1"
set "HTTP_STATUS="
if defined AUTH_HEADER (
    curl -# -T "%FILEPATH%" -H "%AUTH_HEADER%" "%UPLOAD_URL%" -o "%TMP_RESPONSE%" -w "%{http_code}" > "%TMP_STATUS%"
) else (
    curl -# -T "%FILEPATH%" "%UPLOAD_URL%" -o "%TMP_RESPONSE%" -w "%{http_code}" > "%TMP_STATUS%"
)
if exist "%TMP_STATUS%" (
    set /p "HTTP_STATUS="<"%TMP_STATUS%"
)
exit /b 0

:CHECK_AND_NORMALIZE
set "INPUT=%~1"
for %%I in ("%INPUT%") do (
    set "FILEPATH=%%~fI"
    set "FILENAME=%%~nxI"
)
if not exist "%FILEPATH%" (
    echo Error: File not found: %INPUT%
    exit /b 1
)
exit /b 0

:SAVE_SETTINGS
>"%SETTINGS_FILE%" (
    echo account=%ACCOUNT_ID%
    echo default=%DEFAULT_MODE%
    echo last_mode=%LAST_MODE%
)
exit /b 0

:PAUSE
echo.
pause
goto :eof

rem bye
:QUIT
endlocal & exit
