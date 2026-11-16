# Run the review probes through a temporary Go overlay without editing http/.
# These tests intentionally fail while the documented defects remain present.
$ErrorActionPreference = 'Stop'
$reviewRepository = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
$reviewOverlayPath = [System.IO.Path]::GetTempFileName()
$reviewExitCode = 1
$reviewOverlay = @{
    Replace = @{
        (Join-Path $reviewRepository 'http/review_test.go') = (Join-Path $PSScriptRoot 'http_review_test.go.txt')
    }
}

try {
    [System.IO.File]::WriteAllText(
        $reviewOverlayPath,
        ($reviewOverlay | ConvertTo-Json -Depth 4),
        [System.Text.UTF8Encoding]::new($false)
    )
    Push-Location -LiteralPath $reviewRepository
    try {
        & go test -overlay $reviewOverlayPath ./http -run '^TestReview' -v -timeout 30s
        $reviewExitCode = $LASTEXITCODE
    }
    finally {
        Pop-Location
    }
}
finally {
    Remove-Item -LiteralPath $reviewOverlayPath -Force
}

exit $reviewExitCode
