package commands

import (
	stores "backend/stores"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MKUSR estructura que representa el comando mkusr con sus parámetros
type MKUSR struct {
	user string // Nombre del usuario
	pass string // Contraseña del usuario
	grp  string // Grupo del usuario
}

// ParseMkusr (sin cambios)
func ParseMkusr(tokens []string) (string, error) {
	cmd := &MKUSR{}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-user=[^\s]+|(?i)-pass=[^\s]+|(?i)-grp=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-user": true,
		"-pass": true,
		"-grp":  true,
	}

	for _, param := range allParams {
		// Extraer solo el nombre del parámetro (sin el valor) y convertir a minúsculas
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		if !allowedParams[paramName] {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	for _, match := range matches {
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		// Convertir el nombre del parámetro a minúsculas para la comparación
		key, value := strings.ToLower(kv[0]), kv[1]

		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		switch key {
		case "-user":
			if value == "" {
				return "", errors.New("el nombre del usuario no puede estar vacío")
			}
			if len(value) > 10 {
				return "", errors.New("el nombre del usuario no puede exceder 10 caracteres")
			}
			cmd.user = value
		case "-pass":
			if value == "" {
				return "", errors.New("la contraseña no puede estar vacía")
			}
			if len(value) > 10 {
				return "", errors.New("la contraseña no puede exceder 10 caracteres")
			}
			cmd.pass = value
		case "-grp":
			if value == "" {
				return "", errors.New("el nombre del grupo no puede estar vacío")
			}
			if len(value) > 10 {
				return "", errors.New("el nombre del grupo no puede exceder 10 caracteres")
			}
			cmd.grp = value
		}
	}

	if cmd.user == "" || cmd.pass == "" || cmd.grp == "" {
		return "", errors.New("faltan parámetros requeridos: -user, -pass, -grp")
	}

	err := commandMkusr(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MKUSR: Usuario creado exitosamente\n-> Usuario: %s\n-> Grupo: %s", cmd.user, cmd.grp), nil
}

// commandMkusr ejecuta la lógica para crear un usuario en users.txt
func commandMkusr(mkusr *MKUSR) error {
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para crear un usuario")
	}

	if stores.ActiveSession.Username != "root" {
		return errors.New("solo el usuario root puede crear usuarios")
	}

	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return err
	}

	usersText, err := sb.ReadUsersFile(partition, path)
	if err != nil {
		return err
	}

	lines := strings.Split(usersText, "\n")
	newUID := int32(1)
	var groupGID int32 = -1
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			continue
		}
		typ := strings.TrimSpace(fields[1])

		if typ == "U" && len(fields) == 5 {
			userName := strings.TrimSpace(fields[2]) // Cambiar a fields[2] porque ahora el nombre está en la posición correcta
			if userName == mkusr.user {
				return errors.New("el usuario ya existe")
			}
			uid := int32(0)
			fmt.Sscanf(strings.TrimSpace(fields[0]), "%d", &uid)
			if uid >= newUID {
				newUID = uid + 1
			}
		}

		if typ == "G" {
			groupName := strings.TrimSpace(fields[2])
			if groupName == mkusr.grp {
				gid := int32(0)
				fmt.Sscanf(strings.TrimSpace(fields[0]), "%d", &gid)
				if gid == 0 {
					return errors.New("el grupo especificado está eliminado")
				}
				groupGID = gid
			}
		}
	}

	if groupGID == -1 {
		return errors.New("el grupo no existe en la partición")
	}

	// Formato correcto: "ID,U,Nombre,Grupo,Contraseña"
	newUserEntry := fmt.Sprintf("%d,U,%s,%s,%s", newUID, mkusr.user, mkusr.grp, mkusr.pass)
	newUsersText := usersText
	if !strings.HasSuffix(newUsersText, "\n") {
		newUsersText += "\n"
	}
	newUsersText += newUserEntry

	err = sb.UpdateUsersFile(partition, path, newUsersText)
	if err != nil {
		return err
	}

	return nil
}
