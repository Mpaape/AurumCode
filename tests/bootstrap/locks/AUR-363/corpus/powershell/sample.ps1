# Bounded, non-sensitive fixture corpus for the AUR-363 PowerShell adapter
# lock. It exists only to be digested, never parsed.

function Greet {
    <#
    .SYNOPSIS
    Returns a static greeting.
    #>
    param([string]$Name)
    "hello, $Name"
}
