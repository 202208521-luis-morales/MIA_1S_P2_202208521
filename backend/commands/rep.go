package commands

import (
	"backend/reports"
	stores "backend/stores"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// REP estructura que representa el comando rep con sus parámetros
type REP struct {
	id           string // ID del disco
	path         string // Ruta del archivo del disco
	name         string // Nombre del reporte
	path_file_ls string // Ruta del archivo ls (opcional)
}

// ParserRep parsea el comando rep y devuelve una instancia de REP
func ParseRep(tokens []string) (string, error) {
	cmd := &REP{} // Crea una nueva instancia de REP

	// Unir tokens en una sola cadena y luego dividir por espacios, respetando las comillas
	args := strings.Join(tokens, " ")
	// Expresión regular para encontrar los parámetros del comando rep, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-id=[^\s]+|(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-name=[^\s]+|(?i)-path_file_ls="[^"]+"|(?i)-path_file_ls=[^\s]+`)
	// Encuentra todas las coincidencias de la expresión regular en la cadena de argumentos
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-id":           true,
		"-path":         true,
		"-name":         true,
		"-path_file_ls": true,
	}

	for _, param := range allParams {
		// Extraer solo el nombre del parámetro (sin el valor) y convertir a minúsculas
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		if !allowedParams[paramName] {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	// Itera sobre cada coincidencia encontrada
	for _, match := range matches {
		// Divide cada parte en clave y valor usando "=" como delimitador
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		key, value := strings.ToLower(kv[0]), kv[1]

		// Remove quotes from value if present
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		// Switch para manejar diferentes parámetros
		switch key {
		case "-id":
			// Verifica que el id no esté vacío
			if value == "" {
				return "", errors.New("el id no puede estar vacío")
			}
			cmd.id = value
		case "-path":
			// Verifica que el path no esté vacío
			if value == "" {
				return "", errors.New("el path no puede estar vacío")
			}
			cmd.path = value
		case "-name":
			// Verifica que el nombre sea uno de los valores permitidos
			validNames := []string{"mbr", "disk", "inode", "block", "tree", "bm_inode", "bm_block", "sb", "file", "ls"}
			if !contains(validNames, value) {
				return "", errors.New("nombre inválido, debe ser uno de los siguientes: mbr, disk, inode, block, bm_inode, bm_block, sb, file, ls")
			}
			cmd.name = value
		case "-path_file_ls":
			cmd.path_file_ls = value
		}
	}

	// Verifica que los parámetros obligatorios hayan sido proporcionados
	if cmd.id == "" || cmd.path == "" || cmd.name == "" {
		return "", errors.New("faltan parámetros requeridos: -id, -path, -name")
	}

	// Aquí se puede agregar la lógica para ejecutar el comando rep con los parámetros proporcionados
	err := commandRep(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("REP: Reporte generado exitosamente\n"+
		"-> ID: %s\n"+
		"-> Path: %s\n"+
		"-> Tipo: %s%s",
		cmd.id,
		cmd.path,
		cmd.name,
		func() string {
			if cmd.path_file_ls != "" {
				return fmt.Sprintf("\n-> Path LS: %s", cmd.path_file_ls)
			}
			return ""
		}()), nil
}

// Función auxiliar para verificar si un valor está en una lista
func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// commandRep ejecuta el comando rep con los parámetros proporcionados
func commandRep(rep *REP) error {
	// Obtener la partición montada
	mountedMbr, mountedSb, mountedDiskPath, err := stores.GetMountedPartitionRep(rep.id)
	if err != nil {
		return err
	}

	// Switch para manejar diferentes tipos de reportes
	switch rep.name {
	case "mbr":
		err = reports.ReportMBR(mountedMbr, rep.path)
		if err != nil {
			return fmt.Errorf("error generando reporte MBR: %v", err)
		}
	case "disk":
		err = reports.ReportDisk(mountedMbr, mountedDiskPath, rep.path)
		if err != nil {
			return fmt.Errorf("error generando reporte DISK: %v", err)
		}
	case "inode":
		err = reports.ReportInode(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			return fmt.Errorf("error generando reporte INODE: %v", err)
		}
	case "bm_inode":
		err = reports.ReportBMInode(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			return fmt.Errorf("error generando reporte BM_INODE: %v", err)
		}
	case "block":
		err = reports.ReportBlock(mountedDiskPath, rep.id, mountedSb, rep.path)
		if err != nil {
			return fmt.Errorf("error generando reporte BLOCK: %v", err)
		}
	case "sb":
		err = reports.ReportSB(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			return fmt.Errorf("error generando reporte SB: %v", err)
		}
	case "file":
		err = reports.ReportFile(mountedSb, mountedDiskPath, rep.path_file_ls, rep.path)
		if err != nil {
			return fmt.Errorf("error generando reporte FILE: %v", err)
		}
	case "ls":
		err = reports.ReportLS(mountedSb, mountedDiskPath, rep.path_file_ls, rep.path)
		if err != nil {
			return fmt.Errorf("error generando reporte LS: %v", err)
		}
	case "bm_block":
		err = reports.ReportBMBloc(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			return fmt.Errorf("error al generar el reporte bm_bloc: %v", err)
		}
	case "tree":
		err = reports.ReportTree(mountedSb, mountedDiskPath, rep.path)
		if err != nil {
			return fmt.Errorf("error al generar el reporte tree: %v", err)
		}
	default:
		return fmt.Errorf("reporte no soportado: %s", rep.name)
	}

	return nil
}
