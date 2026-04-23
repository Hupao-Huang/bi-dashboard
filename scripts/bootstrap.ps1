<#
.SYNOPSIS
BI 看板开发环境一键恢复脚本

.DESCRIPTION
在新机器上从 Git 克隆仓库后运行此脚本，自动完成：
  1. 开发环境检查（git/node/npm/go/mysql）
  2. 验证 server/config.json 是否就位
  3. npm install 前端依赖
  4. go mod download + 编译所有 Go 二进制
  5. 可选：从 mysqldump 文件恢复 MySQL 数据库

.PARAMETER DumpPath
MySQL dump 文件路径。支持 .sql / .sql.gz / .sql.zst 三种格式。
不指定则跳过数据库恢复。

.PARAMETER SkipFrontend
跳过 npm install（加快只恢复后端的情况）

.PARAMETER SkipBuild
跳过 Go 编译（加快只恢复前端/数据库的情况）

.EXAMPLE
.\scripts\bootstrap.ps1
首次恢复（跳过数据库，后续手动导入）

.EXAMPLE
.\scripts\bootstrap.ps1 -DumpPath C:\backup\bi_dashboard.sql.zst
完整恢复（包括数据库）

.EXAMPLE
.\scripts\bootstrap.ps1 -SkipFrontend -SkipBuild -DumpPath C:\backup\x.sql
只恢复数据库

.NOTES
前置条件：
  - 已 git clone bi-dashboard 仓库到本机
  - 手动拷贝原机器的 server\config.json 到本机对应位置
  - 本机已装：Git、Node.js 18+、Go 1.24+、MySQL 8+
#>
#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$DumpPath,
    [switch]$SkipFrontend,
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'

function Step($msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
function OK($msg)   { Write-Host "    [OK] $msg" -ForegroundColor Green }
function Warn($msg) { Write-Host "    [!]  $msg" -ForegroundColor Yellow }
function Die($msg)  { Write-Host "    [X]  $msg" -ForegroundColor Red; exit 1 }

# ---- 定位仓库根 ----
$root = Resolve-Path (Join-Path $PSScriptRoot '..')
Set-Location $root
Write-Host "仓库根目录: $root" -ForegroundColor Gray
Write-Host "DumpPath:   $(if ($DumpPath) { $DumpPath } else { '(未指定，跳过数据库恢复)' })" -ForegroundColor Gray

# ---- 1/5 环境检查 ----
Step '1/5 检查开发环境'
# Go 用 "go version"（不带 --），其他用 --version
$required = @(
    @{ Name='git';   Args=@('--version') },
    @{ Name='node';  Args=@('--version') },
    @{ Name='npm';   Args=@('--version') },
    @{ Name='go';    Args=@('version') }
)
if ($DumpPath) { $required += @{ Name='mysql'; Args=@('--version') } }
$missing = @()
foreach ($tool in $required) {
    $c = Get-Command $tool.Name -ErrorAction SilentlyContinue
    if ($null -eq $c) {
        $missing += $tool.Name
        Warn "$($tool.Name) : 未找到"
    } else {
        $ver = (& $tool.Name @($tool.Args) 2>&1 | Select-Object -First 1) -as [string]
        OK "$($tool.Name) : $ver"
    }
}
if ($missing.Count) {
    Die "缺少依赖: $($missing -join ', ')。请先安装（推荐用 winget 或 scoop）。"
}

# ---- 2/5 仓库状态 ----
Step '2/5 检查 Git 仓库状态'
if (-not (Test-Path '.git')) { Die '当前目录不是 Git 仓库' }
$branch = git rev-parse --abbrev-ref HEAD 2>$null
$commit = git rev-parse --short HEAD 2>$null
OK "分支=$branch  HEAD=$commit"

# ---- 3/5 config.json ----
Step '3/5 检查 server\config.json'
$cfgPath = Join-Path $root 'server\config.json'
if (-not (Test-Path $cfgPath)) {
    Warn 'server\config.json 不存在'
    Write-Host ''
    Write-Host '  此文件已被 .gitignore 排除（含密钥，不能进仓库）。' -ForegroundColor Yellow
    Write-Host '  请从原机器拷贝过来，位置：server\config.json' -ForegroundColor Yellow
    Write-Host '  包含字段：MySQL 密码 / 吉客云 AppKey / 钉钉 Secret / 合思 Secret / Webhook Secret' -ForegroundColor Yellow
    Write-Host ''
    Die '请先准备 config.json 后重新运行'
}
# 快速校验 JSON 格式
try {
    $cfg = Get-Content $cfgPath -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not $cfg.database.host -or -not $cfg.database.user -or -not $cfg.database.dbname) {
        Die 'config.json 中 database 配置不完整（需要 host/user/dbname）'
    }
    OK "config.json 就位 (db=$($cfg.database.host):$($cfg.database.port)/$($cfg.database.dbname))"
} catch {
    Die "config.json 格式错误: $_"
}

# ---- 4/5 前端依赖 ----
if ($SkipFrontend) {
    Step '4/5 跳过前端依赖 (-SkipFrontend)'
} else {
    Step '4/5 安装前端依赖 (npm install, 约 3-5 分钟)'
    npm install --no-fund --no-audit
    if ($LASTEXITCODE) { Die 'npm install 失败' }
    OK '前端依赖已安装'
}

# ---- 5/5 后端编译 ----
if ($SkipBuild) {
    Step '5/5 跳过后端编译 (-SkipBuild)'
} else {
    Step '5/5 编译后端 Go 二进制'
    Push-Location 'server'
    try {
        Write-Host '    拉取 Go 依赖 (go mod download)...' -NoNewline
        go mod download 2>&1 | Out-Null
        if ($LASTEXITCODE) { Write-Host ' FAIL' -ForegroundColor Red; Die 'go mod download 失败' }
        Write-Host ' OK' -ForegroundColor Green

        # 主服务
        Write-Host '    编译 bi-server.exe...' -NoNewline
        go build -o 'bi-server.exe' './cmd/server' 2>&1 | Out-Null
        if ($LASTEXITCODE) { Write-Host ' FAIL' -ForegroundColor Red; Die '主服务编译失败，请检查 go 版本和源码' }
        Write-Host ' OK' -ForegroundColor Green

        # 生产工具（只编译 import-* / sync-* / snapshot-*，跳过 benchmark/check/debug/probe/test 等开发工具）
        $prodDirs = Get-ChildItem 'cmd' -Directory | Where-Object {
            $_.Name -match '^(import|sync|snapshot)-' -and (Test-Path (Join-Path $_.FullName 'main.go'))
        }
        $built = 0; $failed = @()
        foreach ($d in $prodDirs) {
            $exe = "$($d.Name).exe"
            Write-Host "    编译 $exe..." -NoNewline
            go build -o $exe "./cmd/$($d.Name)" 2>&1 | Out-Null
            if ($LASTEXITCODE) {
                Write-Host ' FAIL' -ForegroundColor Red
                $failed += $d.Name
            } else {
                Write-Host ' OK' -ForegroundColor Green
                $built++
            }
        }
        OK "生产工具编译完成：$built 个成功$(if ($failed.Count) {", $($failed.Count) 个失败 ($($failed -join ', '))" })"
    } finally { Pop-Location }
}

# ---- 可选：数据库恢复 ----
if ($DumpPath) {
    Step "恢复 MySQL 数据库 ($DumpPath)"
    if (-not (Test-Path $DumpPath)) { Die "DumpPath 不存在: $DumpPath" }
    $dumpSize = [Math]::Round((Get-Item $DumpPath).Length / 1MB, 1)
    Write-Host "    文件大小: $dumpSize MB" -ForegroundColor Gray
    Write-Host "    目标: $($cfg.database.host):$($cfg.database.port)/$($cfg.database.dbname) (user=$($cfg.database.user))" -ForegroundColor Gray
    Warn '大数据集导入预计 10-60 分钟（视 dump 大小），请耐心等待'

    # 通过 MYSQL_PWD env 传密码（比命令行 -p 安全）
    $env:MYSQL_PWD = $cfg.database.password
    try {
        # 1. 建库（如果不存在）
        $createSql = "CREATE DATABASE IF NOT EXISTS ``$($cfg.database.dbname)`` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
        & mysql -h $cfg.database.host -P $cfg.database.port -u $cfg.database.user -e $createSql 2>&1 | ForEach-Object {
            if ($_ -notmatch 'Using a password') { Write-Host "    $_" -ForegroundColor Gray }
        }
        if ($LASTEXITCODE) { Die 'CREATE DATABASE 失败' }
        OK "数据库 $($cfg.database.dbname) 已就绪"

        # 2. 根据文件扩展名选择导入方式
        $sw = [Diagnostics.Stopwatch]::StartNew()
        $mysqlArgs = @(
            '-h', $cfg.database.host,
            '-P', $cfg.database.port,
            '-u', $cfg.database.user,
            '--default-character-set=utf8mb4',
            $cfg.database.dbname
        )
        if ($DumpPath -like '*.zst') {
            if (-not (Get-Command zstd -ErrorAction SilentlyContinue)) { Die 'zstd 未安装，无法解压 .zst 文件' }
            & cmd /c "zstd -dc `"$DumpPath`" | mysql $($mysqlArgs -join ' ')"
        } elseif ($DumpPath -like '*.gz') {
            if (-not (Get-Command gzip -ErrorAction SilentlyContinue)) { Die 'gzip 未安装，无法解压 .gz 文件' }
            & cmd /c "gzip -dc `"$DumpPath`" | mysql $($mysqlArgs -join ' ')"
        } else {
            & cmd /c "mysql $($mysqlArgs -join ' ') < `"$DumpPath`""
        }
        if ($LASTEXITCODE) { Die '数据库导入失败' }
        $sw.Stop()
        OK "数据库导入完成，耗时 $([int]$sw.Elapsed.TotalMinutes) 分 $($sw.Elapsed.Seconds) 秒"

        # 3. 校验：表数量
        $tableCount = & mysql -h $cfg.database.host -P $cfg.database.port -u $cfg.database.user -N -B -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='$($cfg.database.dbname)';" 2>$null
        OK "已恢复 $tableCount 张表"
    } finally {
        $env:MYSQL_PWD = $null
    }
} else {
    Write-Host ''
    Write-Host '(未指定 -DumpPath，跳过数据库恢复)' -ForegroundColor Gray
    Write-Host '  后续手动恢复：' -ForegroundColor Gray
    Write-Host '    # 原机器导出' -ForegroundColor Gray
    Write-Host "    mysqldump --single-transaction --routines --triggers --default-character-set=utf8mb4 -uroot -p bi_dashboard | zstd -19 > bi_dashboard.sql.zst" -ForegroundColor Gray
    Write-Host '    # 新机器导入' -ForegroundColor Gray
    Write-Host '    .\scripts\bootstrap.ps1 -SkipFrontend -SkipBuild -DumpPath .\bi_dashboard.sql.zst' -ForegroundColor Gray
}

# ---- 完成 ----
Step '恢复完成，下一步'
Write-Host '  启动后端:        cd server; .\bi-server.exe' -ForegroundColor Gray
Write-Host '  前端开发模式:    npm start  (监听 :3000，热更新)' -ForegroundColor Gray
Write-Host '  前端生产模式:    npm run build; npx serve -s build -l 3000' -ForegroundColor Gray
Write-Host '  访问:            http://localhost:3000   (后端 API: :8080)' -ForegroundColor Gray
Write-Host ''
