#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Installs the Syncthing Windows service and firewall rules for the
    syncthing.exe sitting next to this script.

.DESCRIPTION
    Prompts for (or accepts via -User) a local user account that the
    syncthing service should run as, grants that account the
    "Log on as a service" right, stops and removes any existing
    syncthing service, kills any running syncthing.exe and mmc.exe
    (Services snap-in) processes, then creates a new "syncthing"
    service set to start automatically under the chosen account, and
    adds inbound TCP/UDP firewall rules for the executable.

.PARAMETER ServiceName
    Service name. Default: "syncthing".

.PARAMETER Group
    Firewall rule group name. Default: "Syncthing".

.PARAMETER User
    Account to run the service as. Accepted forms:
      * "DOMAIN\user" or "user@domain"        - domain account
      * "COMPUTERNAME\user" or bare "user"    - local account
      * "LocalSystem" / "NetworkService" /
        "LocalService"                        - built-in account
                                               (no password required)
    If omitted, an interactive picker is shown.

.PARAMETER Password
    SecureString password for -User. If omitted, you are prompted.
    Ignored for built-in service accounts.

    NOTE: For Microsoft-account logins (Windows Hello / PIN), the
    service needs the actual MS-account password, not the PIN.

.PARAMETER Force
    Recreate firewall rules even if they already exist.

.PARAMETER SkipCredentialCheck
    Skip the pre-validation of the password against the local SAM /
    domain. Useful if pre-validation gives a false negative; the
    real validation will still happen inside New-Service.
#>

[CmdletBinding()]
param(
    [string]$ServiceName = "syncthing",
    [string]$Group       = "Syncthing",
    [string]$User,
    [System.Security.SecureString]$Password,
    [switch]$Force,
    [switch]$SkipCredentialCheck
)

$ErrorActionPreference = "Stop"

function Write-Step    ($msg) { Write-Host ""; Write-Host "==> $msg" -ForegroundColor Cyan }
function Write-Info    ($msg) { Write-Host "    $msg" -ForegroundColor Gray }
function Write-Ok      ($msg) { Write-Host "    [ok]    $msg" -ForegroundColor Green }
function Write-Skip    ($msg) { Write-Host "    [skip]  $msg" -ForegroundColor DarkGray }
function Write-Warning2($msg) { Write-Host "    [warn]  $msg" -ForegroundColor Yellow }
function Write-Fail    ($msg) { Write-Host "    [fail]  $msg" -ForegroundColor Red }

function Resolve-AccountName {
    param([Parameter(Mandatory)][string]$Name)

    $n = $Name.Trim()
    if ($n.StartsWith(".\")) {
        $n = "$env:COMPUTERNAME\" + $n.Substring(2)
    }
    elseif ($n -notmatch "\\" -and $n -notmatch "@") {
        $n = "$env:COMPUTERNAME\$n"
    }
    return $n
}

function Get-CurrentAccount {
    try {
        $w = (& whoami.exe) 2>$null
        if ($LASTEXITCODE -eq 0 -and $w) {
            $w = $w.Trim()
            if ($w -match "\\") {
                $parts = $w.Split("\", 2)
                return ("{0}\{1}" -f $parts[0].ToUpper(), $parts[1])
            }
            return $w
        }
    } catch {}
    if ($env:USERDOMAIN -and $env:USERNAME) {
        return "$env:USERDOMAIN\$env:USERNAME"
    }
    return "$env:COMPUTERNAME\$env:USERNAME"
}

function Split-Account {
    param([Parameter(Mandatory)][string]$Account)

    if ($Account -match "^([^\\]+)\\(.+)$") {
        return @{ Domain = $Matches[1]; User = $Matches[2] }
    }
    if ($Account -match "^(.+)@(.+)$") {
        return @{ Domain = $Matches[2]; User = $Matches[1] }
    }
    return @{ Domain = $env:COMPUTERNAME; User = $Account }
}

function Test-AccountCredential {
    param(
        [Parameter(Mandatory)][string]$Account,
        [Parameter(Mandatory)][System.Security.SecureString]$Password
    )

    Add-Type -AssemblyName System.DirectoryServices.AccountManagement -ErrorAction Stop
    $parts = Split-Account -Account $Account
    $isLocal = ($parts.Domain -ieq $env:COMPUTERNAME) -or ($parts.Domain -eq ".")

    $ctxType = if ($isLocal) {
        [System.DirectoryServices.AccountManagement.ContextType]::Machine
    } else {
        [System.DirectoryServices.AccountManagement.ContextType]::Domain
    }

    $bstr = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($Password)
    try {
        $plain = [System.Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
        $ctx = if ($isLocal) {
            New-Object System.DirectoryServices.AccountManagement.PrincipalContext($ctxType)
        } else {
            New-Object System.DirectoryServices.AccountManagement.PrincipalContext($ctxType, $parts.Domain)
        }
        try {
            return $ctx.ValidateCredentials($parts.User, $plain)
        } finally {
            $ctx.Dispose()
        }
    } finally {
        [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    }
}

function Test-IsBuiltinServiceAccount {
    param([Parameter(Mandatory)][string]$Account)
    $a = $Account.Trim().ToLowerInvariant()
    return $a -in @(
        "localsystem",
        "nt authority\system",
        "system",
        "nt authority\networkservice",
        "networkservice",
        "network service",
        "nt authority\localservice",
        "localservice",
        "local service"
    )
}

function Resolve-BuiltinServiceAccount {
    param([Parameter(Mandatory)][string]$Account)
    switch ($Account.Trim().ToLowerInvariant()) {
        "localsystem"                   { return "LocalSystem" }
        "system"                        { return "LocalSystem" }
        "nt authority\system"           { return "LocalSystem" }
        "networkservice"                { return "NT AUTHORITY\NetworkService" }
        "network service"               { return "NT AUTHORITY\NetworkService" }
        "nt authority\networkservice"   { return "NT AUTHORITY\NetworkService" }
        "localservice"                  { return "NT AUTHORITY\LocalService" }
        "local service"                 { return "NT AUTHORITY\LocalService" }
        "nt authority\localservice"     { return "NT AUTHORITY\LocalService" }
    }
    return $Account
}

function Select-ServiceAccount {
    $current = Get-CurrentAccount
    $users   = @(Get-LocalUser -ErrorAction SilentlyContinue | Where-Object { $_.Enabled })

    Write-Info "Current account (from whoami): $current"
    Write-Host ""
    Write-Info "Built-in service accounts (no password):"
    Write-Host "      [S] LocalSystem            (full machine access; no user profile)" -ForegroundColor White
    Write-Host "      [N] NT AUTHORITY\NetworkService" -ForegroundColor White
    Write-Host "      [L] NT AUTHORITY\LocalService" -ForegroundColor White
    if ($users -and $users.Count -gt 0) {
        Write-Host ""
        Write-Info "Enabled local accounts on '$env:COMPUTERNAME':"
        for ($i = 0; $i -lt $users.Count; $i++) {
            Write-Host ("      [{0}] {1}\{2}" -f ($i + 1), $env:COMPUTERNAME, $users[$i].Name) -ForegroundColor White
        }
    }
    Write-Host ""

    while ($true) {
        $prompt = if ($users -and $users.Count -gt 0) {
            "    Enter to use '$current', pick 1-$($users.Count), S/N/L for built-in, or type DOMAIN\user"
        } else {
            "    Enter to use '$current', S/N/L for built-in, or type DOMAIN\user"
        }
        $answer = Read-Host $prompt

        if ([string]::IsNullOrWhiteSpace($answer)) {
            return $current
        }
        switch ($answer.Trim().ToUpperInvariant()) {
            "S" { return "LocalSystem" }
            "N" { return "NT AUTHORITY\NetworkService" }
            "L" { return "NT AUTHORITY\LocalService" }
        }
        $idx = 0
        if ($users -and [int]::TryParse($answer, [ref]$idx) -and $idx -ge 1 -and $idx -le $users.Count) {
            return "$env:COMPUTERNAME\$($users[$idx - 1].Name)"
        }
        if ($answer.Trim().Length -gt 0) {
            return Resolve-AccountName -Name $answer
        }
        Write-Warning2 "Invalid selection, try again"
    }
}

function Grant-LogonAsService {
    param([Parameter(Mandatory)][string]$AccountName)

    $sid = (New-Object System.Security.Principal.NTAccount($AccountName)).
            Translate([System.Security.Principal.SecurityIdentifier]).Value
    $entry = "*$sid"

    $cfg = [System.IO.Path]::GetTempFileName()
    $sdb = [System.IO.Path]::GetTempFileName()
    try {
        & secedit.exe /export /cfg $cfg /areas USER_RIGHTS | Out-Null

        $lines   = Get-Content -LiteralPath $cfg
        $key     = "SeServiceLogonRight"
        $found   = $false
        $changed = $false

        for ($i = 0; $i -lt $lines.Count; $i++) {
            if ($lines[$i] -match "^\s*$key\s*=") {
                $found = $true
                if ($lines[$i] -notmatch [regex]::Escape($entry)) {
                    $lines[$i] = $lines[$i].TrimEnd() + ",$entry"
                    $changed = $true
                }
                break
            }
        }
        if (-not $found) {
            $hit = $lines | Select-String -SimpleMatch "[Privilege Rights]" | Select-Object -First 1
            if (-not $hit) {
                throw "Could not locate [Privilege Rights] section in exported security policy."
            }
            $insertAt = $hit.LineNumber
            $lines = $lines[0..($insertAt - 1)] + "$key = $entry" + $lines[$insertAt..($lines.Count - 1)]
            $changed = $true
        }

        if (-not $changed) { return $false }

        Set-Content -LiteralPath $cfg -Value $lines -Encoding Unicode
        & secedit.exe /import /db $sdb /cfg $cfg /areas USER_RIGHTS | Out-Null
        & secedit.exe /configure /db $sdb /areas USER_RIGHTS | Out-Null
        return $true
    }
    finally {
        Remove-Item -LiteralPath $cfg -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $sdb -ErrorAction SilentlyContinue
    }
}

try {
    Write-Host ""
    Write-Host "  Syncthing service installer" -ForegroundColor White
    Write-Host "  ---------------------------" -ForegroundColor DarkGray

    # --- Resolve executable ---
    Write-Step "Resolving executable"
    $ExePath = Join-Path $PSScriptRoot "syncthing.exe"
    if (-not (Test-Path -LiteralPath $ExePath)) {
        Write-Fail "syncthing.exe not found next to this script."
        Write-Info "Looked in: $PSScriptRoot"
        return
    }
    $resolved = (Resolve-Path -LiteralPath $ExePath).ProviderPath
    Write-Ok $resolved

    # --- Service account ---
    Write-Step "Choosing service account"
    if (-not $User) {
        $Account = Select-ServiceAccount
    } elseif (Test-IsBuiltinServiceAccount $User) {
        $Account = Resolve-BuiltinServiceAccount $User
    } else {
        $Account = Resolve-AccountName -Name $User
    }
    $isBuiltin = Test-IsBuiltinServiceAccount $Account
    Write-Ok "Account: $Account"

    $serviceCred = $null
    if ($isBuiltin) {
        Write-Skip "Built-in service account — no password needed, no logon-right grant needed"
    } else {
        $passwordProvided = [bool]$Password
        $maxAttempts = if ($passwordProvided) { 1 } else { 3 }
        for ($attempt = 1; $attempt -le $maxAttempts; $attempt++) {
            if (-not $Password) {
                $Password = Read-Host -AsSecureString "    Password for '$Account'"
            }
            if (-not $Password -or $Password.Length -eq 0) {
                Write-Fail "Empty password — aborting."
                return
            }

            if ($SkipCredentialCheck) {
                $serviceCred = New-Object System.Management.Automation.PSCredential($Account, $Password)
                Write-Warning2 "Credential pre-check skipped (-SkipCredentialCheck); New-Service will validate"
                break
            }

            try {
                $ok = Test-AccountCredential -Account $Account -Password $Password
            } catch {
                Write-Warning2 "Could not pre-validate credentials ($($_.Exception.Message)). Continuing — New-Service will validate."
                $ok = $true
            }

            if ($ok) {
                $serviceCred = New-Object System.Management.Automation.PSCredential($Account, $Password)
                Write-Ok "Credentials validated"
                break
            }

            Write-Fail "Logon failure: wrong password or account name is not what Windows expects."
            Write-Info "Tips:"
            Write-Info "  - If you sign in with a Microsoft account using a PIN/Hello, the"
            Write-Info "    service needs the actual MS-account password, not the PIN."
            Write-Info "    Reset/check it at: https://account.microsoft.com/security"
            Write-Info "  - Or rerun with -SkipCredentialCheck to bypass this gate."
            Write-Info "  - Or pick a built-in account (no password):"
            Write-Info "      -User LocalSystem      (full machine access; no user profile)"
            Write-Info "      -User NetworkService"
            Write-Info "      -User LocalService"
            Write-Info "  - Verify the account name with: whoami"
            Write-Info "  - Check Caps Lock / keyboard layout."
            if ($attempt -lt $maxAttempts) {
                Write-Info "Attempt $attempt of $maxAttempts failed, try again."
                $Password = $null
            } else {
                Write-Fail "Giving up after $maxAttempts attempt(s)."
                return
            }
        }
    }

    # --- Grant 'Log on as a service' right ---
    if (-not $isBuiltin) {
        Write-Step "Granting 'Log on as a service' right to '$Account'"
        if (Grant-LogonAsService -AccountName $Account) {
            Write-Ok "Right granted"
        } else {
            Write-Skip "Right already present"
        }
    }

    # --- 1. Kill mmc.exe (Services snap-in holds locks on the service) ---
    Write-Step "Stopping mmc.exe (Services snap-in)"
    $mmc = Get-Process -Name "mmc" -ErrorAction SilentlyContinue
    if ($mmc) {
        $mmc | Stop-Process -Force -ErrorAction SilentlyContinue
        Write-Ok "Stopped $($mmc.Count) process(es)"
    } else {
        Write-Skip "No mmc.exe running"
    }

    # --- 2. Kill any running syncthing.exe ---
    Write-Step "Stopping syncthing.exe"
    $st = Get-Process -Name "syncthing" -ErrorAction SilentlyContinue
    if ($st) {
        $st | Stop-Process -Force -ErrorAction SilentlyContinue
        Write-Ok "Stopped $($st.Count) process(es)"
    } else {
        Write-Skip "No syncthing.exe running"
    }

    # --- 3. Stop and delete the existing service, if present ---
    Write-Step "Removing existing '$ServiceName' service"
    $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($svc) {
        if ($svc.Status -ne "Stopped") {
            Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
            (Get-Service -Name $ServiceName).WaitForStatus("Stopped", "00:00:30")
            Write-Ok "Stopped"
        }
        & sc.exe delete $ServiceName | Out-Null
        Start-Sleep -Seconds 1
        Write-Ok "Deleted"
    } else {
        Write-Skip "Service not installed"
    }

    # --- 4. Create the service ---
    Write-Step "Creating '$ServiceName' service"
    $binPath = "`"$resolved`" --no-browser"
    Write-Info "binPath: $binPath"
    Write-Info "runAs:   $Account"
    if ($isBuiltin) {
        if ($Account -ieq "LocalSystem") {
            New-Service `
                -Name           $ServiceName `
                -BinaryPathName $binPath `
                -DisplayName    $ServiceName `
                -StartupType    Automatic | Out-Null
        } else {
            & sc.exe create $ServiceName binPath= $binPath start= auto obj= $Account DisplayName= $ServiceName | Out-Null
            if ($LASTEXITCODE -ne 0) {
                throw "sc.exe create failed with exit code $LASTEXITCODE"
            }
        }
    } else {
        New-Service `
            -Name           $ServiceName `
            -BinaryPathName $binPath `
            -DisplayName    $ServiceName `
            -StartupType    Automatic `
            -Credential     $serviceCred | Out-Null
    }
    Write-Ok "Created (start=auto, runAs=$Account)"

    # --- 5. Firewall rules ---
    Write-Step "Configuring firewall rules (group '$Group')"
    $existing = Get-NetFirewallRule -Group $Group -ErrorAction SilentlyContinue
    if ($existing -and -not $Force) {
        Write-Skip "$($existing.Count) rule(s) already exist (pass -Force to recreate)"
    } else {
        if ($existing) {
            $existing | Remove-NetFirewallRule
            Write-Ok "Removed $($existing.Count) existing rule(s)"
        }
        $rules = @(
            @{ DisplayName = "Syncthing (TCP-In)"; Protocol = "TCP" },
            @{ DisplayName = "Syncthing (UDP-In)"; Protocol = "UDP" }
        )
        foreach ($r in $rules) {
            New-NetFirewallRule `
                -DisplayName $r.DisplayName `
                -Direction   Inbound `
                -Program     $resolved `
                -Protocol    $r.Protocol `
                -Action      Allow `
                -Profile     Any `
                -Group       $Group | Out-Null
            Write-Ok "$($r.DisplayName) ($($r.Protocol))"
        }
    }

    # --- Summary ---
    Write-Step "Summary"
    Write-Host ""
    Write-Host "  Service" -ForegroundColor White
    Get-CimInstance -ClassName Win32_Service -Filter "Name='$ServiceName'" |
        Format-Table @{L='Name';E={$_.Name}},
                     @{L='State';E={$_.State}},
                     @{L='StartMode';E={$_.StartMode}},
                     @{L='RunAs';E={$_.StartName}},
                     @{L='DisplayName';E={$_.DisplayName}} `
                     -AutoSize | Out-Host

    Write-Host "  Firewall rules" -ForegroundColor White
    Get-NetFirewallRule -Group $Group |
        Format-Table @{L='DisplayName';E={$_.DisplayName}},
                     @{L='Enabled';E={$_.Enabled}},
                     @{L='Direction';E={$_.Direction}},
                     @{L='Action';E={$_.Action}} `
                     -AutoSize | Out-Host

    Write-Host ""
    Write-Host "  Done." -ForegroundColor Green
}
catch {
    Write-Fail $_.Exception.Message
}
finally {
    Read-Host
}
