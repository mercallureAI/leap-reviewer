Write a concise, accurate pull request body based on the current pull request context.
Do not read files outside the current workspace. Do not inspect /nix/store or any other external directories.
Do not use additional agents, subagents, or delegated reviews. Complete the rewrite directly in the current workspace.
If any tool call is denied or any permission request is rejected, continue with the repository content and context you already have instead of stopping.
Do not include any preamble such as "I checked", "I reviewed", "I need to", or similar process narration.
If you would normally explain your reasoning first, omit it and output only the final PR body.
