package commands

import (
	stores "backend/stores"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// CHGRP estructura que representa el comando chgrp con sus parámetros
type CHGRP struct {
	user string // Nombre del usuario
	grp  string // Nombre del nuevo grupo
}

// ParseChgrp parsea el comando chgrp y cambia el grupo del usuario
func ParseChgrp(tokens []string) (string, error) {
	cmd := &CHGRP{}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-user=[^\s]+|(?i)-grp=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-user": true,
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

	if cmd.user == "" || cmd.grp == "" {
		return "", errors.New("faltan parámetros requeridos: -user, -grp")
	}

	err := commandChgrp(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("CHGRP: Grupo cambiado exitosamente\n-> Usuario: %s\n-> Nuevo grupo: %s", cmd.user, cmd.grp), nil
}

// commandChgrp ejecuta la lógica para cambiar el grupo de un usuario en users.txt con debug extremo
func commandChgrp(chgrp *CHGRP) error {
	fmt.Println("\nDEBUG: Iniciando commandChgrp - User:", chgrp.user, "Nuevo grupo:", chgrp.grp)

	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para cambiar el grupo")
	}

	// Verificar que el usuario sea root
	if stores.ActiveSession.Username != "root" {
		return errors.New("solo el usuario root puede cambiar grupos")
	}

	// Obtener la partición montada y su superbloque
	fmt.Println("DEBUG: Obteniendo superbloque y partición para ID:", stores.ActiveSession.PartitionID)
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}
	fmt.Println("DEBUG: Partición encontrada - Path:", path, "Part_start:", partition.Part_start)

	// Leer el contenido actual de users.txt
	fmt.Println("DEBUG: Leyendo users.txt")
	usersText, err := sb.ReadUsersFile(partition, path)
	if err != nil {
		return fmt.Errorf("error al leer users.txt: %v", err)
	}
	fmt.Printf("DEBUG: Contenido actual de users.txt: %q\n", usersText)

	// Parsear líneas y buscar usuario y grupo
	lines := strings.Split(usersText, "\n")
	userFound := false
	groupExists := false
	var newUsersText strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if newUsersText.Len() > 0 {
				newUsersText.WriteString("\n")
			}
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) != 5 && len(fields) != 3 {
			fmt.Println("DEBUG: Línea ignorada por formato inválido:", line)
			newUsersText.WriteString(line + "\n")
			continue
		}
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		fmt.Println("DEBUG: Línea parseada:", fields)

		// Verificar existencia del grupo
		if fields[1] == "G" && fields[2] == chgrp.grp {
			groupExists = true
		}

		// Buscar y actualizar el usuario
		if fields[1] == "U" && fields[2] == chgrp.user {
			userFound = true
			fmt.Println("DEBUG: Usuario encontrado:", fields)
			newLine := fmt.Sprintf("%s,U,%s,%s,%s", fields[0], fields[2], chgrp.grp, fields[4])
			newUsersText.WriteString(newLine + "\n")
			fmt.Println("DEBUG: Nueva línea para usuario:", newLine)
		} else {
			newUsersText.WriteString(line + "\n")
		}
	}

	// Verificar resultados
	if !userFound {
		return errors.New("el usuario no existe en la partición")
	}
	if !groupExists {
		return errors.New("el grupo no existe en la partición")
	}

	// Actualizar users.txt
	fmt.Println("DEBUG: Nuevo contenido de users.txt a escribir:", newUsersText.String())
	err = sb.UpdateUsersFile(partition, path, newUsersText.String())
	if err != nil {
		return fmt.Errorf("error al actualizar users.txt: %v", err)
	}
	fmt.Println("DEBUG: users.txt actualizado exitosamente")

	return nil
}
