package analyzer

import (
	"backend/commands"
	"errors" // Importa el paquete "errors" para manejar errores
	"fmt"

	// Importa el paquete "fmt" para formatear e imprimir texto
	"strings" // Importa el paquete "strings" para manipulación de cadenas
)

// Analyzer analiza el comando de entrada y ejecuta la acción correspondiente
func Analyzer(input string) (string, error) {
	tokens := strings.Fields(input)

	if len(tokens) == 0 {
		return "", errors.New("no se proporcionó ningún comando")
	}

	// Switch para manejar diferentes comandos
	switch strings.ToLower(tokens[0]) {
	case "mkdisk":
		return commands.ParseMkdisk(tokens[1:])
	case "fdisk":
		return commands.ParseFdisk(tokens[1:])
	case "mount":
		return commands.ParseMount(tokens[1:])
	case "mounted":
		return commands.ParseMounted(tokens)
	case "rmdisk": // Nuevo caso para el comando "rmdisk"
		return commands.ParseRmdisk(tokens[1:])
	case "mkfs":
		return commands.ParseMkfs(tokens[1:])
	case "rep":
		return commands.ParseRep(tokens[1:])
	case "mkdir":
		return commands.ParseMkdir(tokens[1:])
	case "login":
		return commands.ParseLogin(tokens[1:])
	case "logout":
		return commands.ParseLogout(tokens)
	case "mkgrp":
		return commands.ParseMkgrp(tokens[1:])
	case "rmgrp":
		return commands.ParseRmgrp(tokens[1:])
	case "mkusr":
		return commands.ParseMkusr(tokens[1:])
	case "rmusr":
		return commands.ParseRmusr(tokens[1:])
	case "chgrp":
		return commands.ParseChgrp(tokens[1:])
	case "mkfile":
		return commands.ParseMkfile(tokens[1:])
	case "cat":
		return commands.ParseCat(tokens[1:])
	case "unmount":
		return commands.ParseUnmount(tokens[1:])
	case "remove":
		return commands.ParseRemove(tokens[1:])
	default:
		return "", fmt.Errorf("comando desconocido: %s", tokens[0])
	}
}
