package commands

import (
	stores "backend/stores"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MKGRP estructura que representa el comando mkgrp con su parámetro
type MKGRP struct {
	name string // Nombre del grupo
}

// ParseMkgrp (sin cambios)
func ParseMkgrp(tokens []string) (string, error) {
	cmd := &MKGRP{}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-name=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-name": true,
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
		case "-name":
			if value == "" {
				return "", errors.New("el nombre del grupo no puede estar vacío")
			}
			if len(value) > 10 {
				return "", errors.New("el nombre del grupo no puede exceder 10 caracteres")
			}
			cmd.name = value
		}
	}

	if cmd.name == "" {
		return "", errors.New("faltan parámetros requeridos: -name")
	}

	err := commandMkgrp(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MKGRP: Grupo creado exitosamente\n-> Nombre: %s", cmd.name), nil
}

// commandMkgrp ejecuta la lógica para crear un grupo en users.txt
func commandMkgrp(mkgrp *MKGRP) error {
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para crear un grupo")
	}

	if stores.ActiveSession.Username != "root" {
		return errors.New("solo el usuario root puede crear grupos")
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
	newGID := int32(1)
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			continue
		}
		typ := strings.TrimSpace(fields[1])
		if typ == "G" {
			gid := int32(0)
			fmt.Sscanf(fields[0], "%d", &gid)
			groupName := strings.TrimSpace(fields[2])
			if strings.EqualFold(groupName, mkgrp.name) {
				return errors.New("el grupo ya existe")
			}
			if gid >= newGID {
				newGID = gid + 1
			}
		}
	}

	// Formato correcto: "ID,G,Nombre"
	newGroupEntry := fmt.Sprintf("%d,G,%s", newGID, mkgrp.name)
	newUsersText := usersText
	if !strings.HasSuffix(newUsersText, "\n") {
		newUsersText += "\n"
	}
	newUsersText += newGroupEntry

	err = sb.UpdateUsersFile(partition, path, newUsersText)
	if err != nil {
		return err
	}

	return nil
}
