; HandyMute installer (Inno Setup 6).
; Installs the app, fetches & installs VB-CABLE if missing, auto-configures the mic->cable
; pass-through (Windows "Listen to this device"), enables start-at-login, and explains the
; one manual step (pointing the meeting app's mic at CABLE Output).

#define AppName "HandyMute"
#define AppVersion "1.0.0"
#define AppExe "handymute.exe"
#define VBCableURL "https://download.vb-audio.com/Download_CABLE/VBCABLE_Driver_Pack43.zip"

[Setup]
AppId={{B7E9D3A2-1C4F-4E8B-9A6D-2F5C7E0A1B33}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=HandyMute contributors
DefaultDirName={autopf}\HandyMute
DefaultGroupName=HandyMute
DisableProgramGroupPage=yes
UninstallDisplayIcon={app}\{#AppExe}
LicenseFile=LICENSE
InfoAfterFile=installer\teams-setup.txt
OutputDir=dist
OutputBaseFilename=HandyMute-Setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible

[Files]
Source: "dist\handymute.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "LICENSE"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\HandyMute"; Filename: "{app}\{#AppExe}"
Name: "{group}\Uninstall HandyMute"; Filename: "{uninstallexe}"

[Run]
; Ordered dependency setup (VB-Cable -> bridge -> autostart) happens in CurStepChanged below.
; This is just the optional launch checkbox on the final page.
Filename: "{app}\{#AppExe}"; Description: "Launch HandyMute now"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{app}\{#AppExe}"; Parameters: "-remove-bridge"; Flags: runhidden; RunOnceId: "RemoveBridge"
Filename: "{app}\{#AppExe}"; Parameters: "-uninstall"; Flags: runhidden; RunOnceId: "RemoveAutostart"

[Code]
function VBCablePresent: Boolean;
begin
  Result := RegKeyExists(HKLM, 'SYSTEM\CurrentControlSet\Services\VBAudioVACMME');
end;

function OnDownloadProgress(const Url, FileName: String; const Progress, ProgressMax: Int64): Boolean;
begin
  Result := True;
end;

// Stop any running copy so its files can be replaced.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var rc: Integer;
begin
  Exec('taskkill.exe', '/F /IM handymute.exe', '', SW_HIDE, ewWaitUntilTerminated, rc);
  Exec('taskkill.exe', '/F /IM handymute-console.exe', '', SW_HIDE, ewWaitUntilTerminated, rc);
  Result := '';
end;

procedure InstallVBCable;
var
  zip, dir, setupExe: String;
  rc: Integer;
begin
  zip := ExpandConstant('{tmp}\vbcable.zip');
  dir := ExpandConstant('{tmp}\vbcable');
  try
    WizardForm.StatusLabel.Caption := 'Downloading VB-CABLE...';
    DownloadTemporaryFile('{#VBCableURL}', 'vbcable.zip', '', @OnDownloadProgress);
  except
    MsgBox('Could not download VB-CABLE automatically (no internet?).' + #13#10 +
           'Please install it from https://vb-audio.com/Cable/ and then relaunch HandyMute.',
           mbInformation, MB_OK);
    Exit;
  end;

  WizardForm.StatusLabel.Caption := 'Installing VB-CABLE driver...';
  Exec('powershell.exe',
       '-NoProfile -NonInteractive -Command "Expand-Archive -Force ''' + zip + ''' ''' + dir + '''"',
       '', SW_HIDE, ewWaitUntilTerminated, rc);

  setupExe := dir + '\VBCABLE_Setup_x64.exe';
  if FileExists(setupExe) then
  begin
    // -i installs the driver; the package is WHQL-signed so no signature prompt.
    Exec(setupExe, '-i', dir, SW_SHOW, ewWaitUntilTerminated, rc);
    Sleep(4000); // give Windows a moment to enumerate the new CABLE endpoints
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var rc: Integer;
begin
  if CurStep <> ssPostInstall then
    Exit;

  if not VBCablePresent then
    InstallVBCable;

  WizardForm.StatusLabel.Caption := 'Configuring microphone pass-through...';
  if not (Exec(ExpandConstant('{app}\{#AppExe}'), '-setup-bridge', '', SW_HIDE, ewWaitUntilTerminated, rc) and (rc = 0)) then
    MsgBox('HandyMute is installed, but the microphone pass-through could not be configured yet ' +
           '(VB-CABLE may need a reboot to appear).' + #13#10 +
           'After rebooting, open HandyMute and it will work, or re-run this installer.',
           mbInformation, MB_OK);

  WizardForm.StatusLabel.Caption := 'Enabling start at login...';
  Exec(ExpandConstant('{app}\{#AppExe}'), '-install', '', SW_HIDE, ewWaitUntilTerminated, rc);
end;

// Stop a running copy before uninstall too.
function InitializeUninstall: Boolean;
var rc: Integer;
begin
  Exec('taskkill.exe', '/F /IM handymute.exe', '', SW_HIDE, ewWaitUntilTerminated, rc);
  Result := True;
end;
