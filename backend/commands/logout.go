package commands

import (
	stores "backend/stores"
	"errors"
)

// ParseLogout parsea el comando logout y cierra la sesión activa
func ParseLogout(tokens []string) (string, error) {
	// Verificar que no se hayan proporcionado parámetros
	if len(tokens) > 1 {
		return "", errors.New("el comando 'logout' no acepta parámetros")
	}

	// Ejecutar el logout
	err := commandLogout()
	if err != nil {
		return "", err
	}

	return "LOGOUT: Sesión cerrada exitosamente", nil
}

// commandLogout ejecuta la lógica para cerrar la sesión
func commandLogout() error {
	// Verificar si hay una sesión activa
	if stores.ActiveSession == nil {
		return errors.New("no hay una sesión activa para cerrar")
	}

	// Cerrar la sesión
	stores.ActiveSession = nil
	return nil
}
