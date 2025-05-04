package commands

import (
	stores "backend/stores"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// RMGRP estructura que representa el comando rmgrp con su parámetro
type RMGRP struct {
	name string // Nombre del grupo a eliminar
}

// ParseRmgrp parsea el comando rmgrp y elimina un grupo
func ParseRmgrp(tokens []string) (string, error) {
	cmd := &RMGRP{}
	args := strings.Join(tokens, " ")
	// Expresión regular insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-name=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-name": true,
	}

	for _, param := range allParams {
		// Extraer solo el nombre del parámetro y convertir a minúsculas
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
		key, value := strings.ToLower(kv[0]), kv[1]

		// Remove quotes from value if present
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

	// Validar parámetro obligatorio
	if cmd.name == "" {
		return "", errors.New("faltan parámetros requeridos: -name")
	}

	// Ejecutar la eliminación del grupo
	err := commandRmgrp(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("RMGRP: Grupo eliminado exitosamente\n"+
		"-> Nombre: %s", cmd.name), nil
}

// commandRmgrp ejecuta la lógica para eliminar un grupo en users.txt
func commandRmgrp(rmgrp *RMGRP) error {
	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para eliminar un grupo")
	}

	// Verificar que el usuario sea root
	if stores.ActiveSession.Username != "root" {
		return errors.New("solo el usuario root puede eliminar grupos")
	}

	// Obtener la partición montada y su superbloque
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return err
	}

	// Leer el contenido actual de users.txt
	usersText, err := sb.ReadUsersFile(partition, path)
	if err != nil {
		return err
	}

	// Buscar el grupo y marcarlo como eliminado
	lines := strings.Split(usersText, "\n")
	found := false
	newUsersText := ""
	for i, line := range lines {
		if line == "" {
			if i != len(lines)-1 { // Evitar eliminar líneas vacías al final
				newUsersText += "\n"
			}
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			newUsersText += line + "\n"
			continue
		}
		typ := strings.TrimSpace(fields[1])
		if typ == "G" {
			groupName := strings.TrimSpace(fields[2])
			if groupName == rmgrp.name {
				if strings.TrimSpace(fields[0]) == "0" {
					return errors.New("el grupo ya está eliminado")
				}
				newUsersText += fmt.Sprintf("0, G, %s\n", rmgrp.name)
				found = true
			} else {
				newUsersText += line + "\n"
			}
		} else {
			newUsersText += line + "\n"
		}
	}

	// Verificar si el grupo fue encontrado
	if !found {
		return errors.New("el grupo no existe en la partición")
	}

	// Actualizar users.txt en la partición
	err = sb.UpdateUsersFile(partition, path, newUsersText)
	if err != nil {
		return err
	}

	return nil
}
