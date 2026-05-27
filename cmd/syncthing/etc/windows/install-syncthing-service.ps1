#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Installs the Syncthing Windows service and firewall rules for the
    syncthing.exe sitting next to this script.

.DESCRIPTION
    Stops and removes any existing syncthing service, kills any running
    syncthing.exe and mmc.exe (Services snap-in) processes, then creates
    a new "syncthing" service set to start automatically, and adds
    inbound TCP/UDP firewall rules for the executable.

.PARAMETER ServiceName
    Service name. Default: "syncthing".

.PARAMETER Group
    Firewall rule group name. Default: "Syncthing".

.PARAMETER Force
    Recreate firewall rules even if they already exist.
#>

[CmdletBinding()]
param(
    [string]$ServiceName = "syncthing",
    [string]$Group       = "Syncthing",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

function Write-Step    ($msg) { Write-Host ""; Write-Host "==> $msg" -ForegroundColor Cyan }
function Write-Info    ($msg) { Write-Host "    $msg" -ForegroundColor Gray }
function Write-Ok      ($msg) { Write-Host "    [ok]    $msg" -ForegroundColor Green }
function Write-Skip    ($msg) { Write-Host "    [skip]  $msg" -ForegroundColor DarkGray }
function Write-Warning2($msg) { Write-Host "    [warn]  $msg" -ForegroundColor Yellow }
function Write-Fail    ($msg) { Write-Host "    [fail]  $msg" -ForegroundColor Red }

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
    & sc.exe create $ServiceName binPath= $binPath start= auto DisplayName= $ServiceName | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "sc.exe create failed with exit code $LASTEXITCODE"
        return
    }
    Write-Ok "Created (start=auto)"

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
    Get-Service -Name $ServiceName |
        Format-Table @{L='Name';E={$_.Name}},
                     @{L='Status';E={$_.Status}},
                     @{L='StartType';E={$_.StartType}},
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
