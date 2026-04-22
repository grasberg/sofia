package agent

import (
	"fmt"
	"strings"
)

// handlePersonaCommand implements the /persona session command.
func (al *AgentLoop) handlePersonaCommand(args []string, sessionKey string) string {
	if len(args) == 0 {
		// Show current persona and list available
		names := al.personaManager.List()
		if len(names) == 0 {
			return "No personas configured. Add personas to agents.defaults.personas in config.json."
		}

		active := al.personaManager.GetActive(sessionKey)
		var sb strings.Builder
		if active != nil {
			fmt.Fprintf(&sb, "Active persona: %s", active.Name)
			if active.Description != "" {
				fmt.Fprintf(&sb, " — %s", active.Description)
			}
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("No active persona (using default).\n\n")
		}
		sb.WriteString("Available personas:\n")
		for _, name := range names {
			p := al.personaManager.GetActive(sessionKey)
			// Look up the persona by name for its description
			_ = al.personaManager.Switch("__peek__", name)
			peeked := al.personaManager.GetActive("__peek__")
			al.personaManager.Clear("__peek__")

			marker := "  "
			if p != nil && p.Name == name {
				marker = "* "
			}
			if peeked != nil && peeked.Description != "" {
				fmt.Fprintf(&sb, "%s%s — %s\n", marker, name, peeked.Description)
			} else {
				fmt.Fprintf(&sb, "%s%s\n", marker, name)
			}
		}
		return sb.String()
	}

	target := args[0]

	if target == "off" {
		al.personaManager.Clear(sessionKey)
		return "Persona cleared. Using default behavior."
	}

	if err := al.personaManager.Switch(sessionKey, target); err != nil {
		names := al.personaManager.List()
		return fmt.Sprintf(
			"Unknown persona %q. Available: %s",
			target, strings.Join(names, ", "),
		)
	}

	p := al.personaManager.GetActive(sessionKey)
	if p != nil && p.Description != "" {
		return fmt.Sprintf("Switched to persona: %s — %s", p.Name, p.Description)
	}
	return fmt.Sprintf("Switched to persona: %s", target)
}

// handleRoleCommand implements the /role session command. It applies a built-in
// role template as a temporary persona for the current session.
func (al *AgentLoop) handleRoleCommand(args []string, sessionKey string) string {
	if len(args) == 0 {
		// List available roles and indicate which is active.
		roles := ListBuiltinRoles()
		active := al.personaManager.GetActive(sessionKey)

		var sb strings.Builder
		if active != nil && strings.HasPrefix(active.Name, "role:") {
			roleName := strings.TrimPrefix(active.Name, "role:")
			fmt.Fprintf(&sb, "Active role: %s\n\n", roleName)
		} else {
			sb.WriteString("No active role.\n\n")
		}
		sb.WriteString("Available roles:\n")
		for _, r := range roles {
			fmt.Fprintf(&sb, "  %s — %s\n", strings.ToLower(r.Name), r.Description)
		}
		sb.WriteString("\nUsage: /role <name> | /role off")
		return sb.String()
	}

	target := strings.ToLower(args[0])

	if target == "off" {
		active := al.personaManager.GetActive(sessionKey)
		if active != nil && strings.HasPrefix(active.Name, "role:") {
			roleName := strings.TrimPrefix(active.Name, "role:")
			al.personaManager.Unregister("role:" + roleName)
		}
		al.personaManager.Clear(sessionKey)
		return "Role cleared. Using default behavior."
	}

	role, ok := GetBuiltinRole(target)
	if !ok {
		names := make([]string, 0, len(BuiltinRoles))
		for k := range BuiltinRoles {
			names = append(names, k)
		}
		return fmt.Sprintf("Unknown role %q. Available: %s", target, strings.Join(names, ", "))
	}

	// Clear any previously registered role persona for this session.
	active := al.personaManager.GetActive(sessionKey)
	if active != nil && strings.HasPrefix(active.Name, "role:") {
		al.personaManager.Unregister(active.Name)
	}

	// Register the role as a temporary persona and switch to it.
	personaName := "role:" + target
	al.personaManager.Register(personaName, &Persona{
		Name:         personaName,
		Description:  role.Description,
		SystemPrompt: role.SystemPrompt,
		AllowedTools: role.SuggestedTools,
	})

	if err := al.personaManager.Switch(sessionKey, personaName); err != nil {
		return fmt.Sprintf("Failed to apply role: %v", err)
	}

	return fmt.Sprintf("Switched to role: %s — %s", role.Name, role.Description)
}
