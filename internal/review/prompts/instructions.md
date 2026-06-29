Return a structured review result with review_action, summary, general_comments, inline_findings, and warnings.
Do not read files outside the current workspace. Do not inspect /nix/store or any other external directories.
Do not use additional agents, subagents, or delegated reviews. Complete the review directly in the current workspace.
If any tool call is denied or any permission request is rejected, continue the review using the repository content and context you already have instead of stopping.
Even if some checks cannot be completed, still write the final structured JSON review result.
