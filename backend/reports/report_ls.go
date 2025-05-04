package reports

import (
	stores "backend/stores"
	structures "backend/structures"
	utils "backend/utils"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ReportLS genera un reporte de tipo "ls" que muestra la información de archivos y carpetas
func ReportLS(sb *structures.SuperBlock, diskPath, dirPath, outputPath string) error {
	// Verificar que haya una sesión activa
	if stores.ActiveSession == nil {
		return fmt.Errorf("debe iniciar sesión primero para listar archivos")
	}
	fmt.Println("DEBUG: Sesión activa - User:", stores.ActiveSession.Username, "ID:", stores.ActiveSession.PartitionID)

	// Crear las carpetas padre si no existen
	err := utils.CreateParentDirs(outputPath)
	if err != nil {
		return err
	}

	// Normalizar la ruta del directorio
	dirPath = filepath.Clean(dirPath)
	if !strings.HasPrefix(dirPath, "/") {
		return fmt.Errorf("la ruta debe ser absoluta (comenzar con /): %s", dirPath)
	}
	fmt.Println("DEBUG: Ruta normalizada:", dirPath)

	// Buscar el inodo del directorio
	dirInode, folderBlock, dirInodeIndex, err := findOrCreateParentDir(sb, nil, diskPath, dirPath, false)
	if err != nil {
		return fmt.Errorf("error al acceder al directorio %s: %v", dirPath, err)
	}
	fmt.Println("DEBUG: Directorio encontrado - Inodo:", dirInodeIndex)

	// Verificar permisos de lectura del directorio
	if stores.ActiveSession.Username != "root" {
		hasPermission := checkReadPermission(dirInode)
		fmt.Println("DEBUG: Verificando permisos de lectura - UserID:", stores.ActiveSession.UserID, "GroupID:", stores.ActiveSession.GroupID, "Permisos:", string(dirInode.I_perm[:]), "Resultado:", hasPermission)
		if !hasPermission {
			return fmt.Errorf("permiso denegado: no tiene permisos de lectura para %s", dirPath)
		}
	} else {
		fmt.Println("DEBUG: Usuario root, permisos de lectura otorgados automáticamente")
	}

	// Generar la tabla del reporte
	dotContent := "digraph G {\n"
	dotContent += "  rankdir=TB;\n"
	dotContent += "  node [shape=plaintext];\n"
	dotContent += "  tbl [label=<\n"
	dotContent += "    <TABLE BORDER=\"1\" CELLBORDER=\"1\" CELLSPACING=\"0\">\n"
	dotContent += "      <TR><TD COLSPAN=\"8\">Reporte de ARCHIVOS y CARPETAS</TD></TR>\n"
	dotContent += "      <TR>\n"
	dotContent += "        <TD>Permisos</TD>\n"
	dotContent += "        <TD>Owner</TD>\n"
	dotContent += "        <TD>Grupo</TD>\n"
	dotContent += "        <TD>Size (en Bytes)</TD>\n"
	dotContent += "        <TD>Fecha</TD>\n"
	dotContent += "        <TD>Hora</TD>\n"
	dotContent += "        <TD>Tipo</TD>\n"
	dotContent += "        <TD>Name</TD>\n"
	dotContent += "      </TR>\n"

	// Listar los archivos y carpetas en el directorio
	for _, content := range folderBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == "" || name == "." || name == ".." {
			continue // Ignorar entradas vacías y referencias a . y ..
		}
		fmt.Println("DEBUG: Procesando entrada - Nombre:", name, "Inodo:", content.B_inodo)

		// Cargar el inodo del archivo o carpeta
		inode := &structures.Inode{}
		err = inode.Deserialize(diskPath, int64(sb.S_inode_start+content.B_inodo*sb.S_inode_size))
		if err != nil {
			return fmt.Errorf("error al cargar inodo %d: %v", content.B_inodo, err)
		}
		fmt.Println("DEBUG: Inodo cargado - Size:", inode.I_size, "Type:", string(inode.I_type[:]))

		// Obtener permisos
		perms := formatPermissions(inode)

		// Obtener propietario y grupo
		owner, group, err := getOwnerAndGroup(sb, diskPath, inode)
		if err != nil {
			return fmt.Errorf("error al obtener propietario y grupo: %v", err)
		}

		// Obtener tamaño
		size := inode.I_size

		// Obtener fecha y hora de modificación
		mtime := time.Unix(int64(inode.I_mtime), 0)
		fecha := mtime.Format("02/01/2006")
		hora := mtime.Format("15:04")

		// Obtener tipo
		tipo := "Archivo"
		if inode.I_type[0] == '1' {
			tipo = "Carpeta"
		}

		// Añadir fila a la tabla
		dotContent += "      <TR>\n"
		dotContent += fmt.Sprintf("        <TD>%s</TD>\n", perms)
		dotContent += fmt.Sprintf("        <TD>%s</TD>\n", owner)
		dotContent += fmt.Sprintf("        <TD>%s</TD>\n", group)
		dotContent += fmt.Sprintf("        <TD>%d</TD>\n", size)
		dotContent += fmt.Sprintf("        <TD>%s</TD>\n", fecha)
		dotContent += fmt.Sprintf("        <TD>%s</TD>\n", hora)
		dotContent += fmt.Sprintf("        <TD>%s</TD>\n", tipo)
		dotContent += fmt.Sprintf("        <TD>%s</TD>\n", name)
		dotContent += "      </TR>\n"
	}

	dotContent += "    </TABLE>\n"
	dotContent += "  >];\n"
	dotContent += "}\n"

	// Escribir el archivo .dot
	dotFilePath := outputPath + ".dot"
	err = os.WriteFile(dotFilePath, []byte(dotContent), 0644)
	if err != nil {
		return fmt.Errorf("error al escribir el archivo .dot: %v", err)
	}

	// Generar la imagen con Graphviz directamente
	cmd := exec.Command("dot", "-Tjpg", dotFilePath, "-o", outputPath)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error al generar la imagen con Graphviz: %v (asegúrate de que Graphviz esté instalado)", err)
	}

	// Opcional: Eliminar el archivo .dot después de generar la imagen
	err = os.Remove(dotFilePath)
	if err != nil {
		fmt.Println("DEBUG: No se pudo eliminar el archivo .dot:", err)
	}

	fmt.Println("DEBUG: Reporte LS generado:", outputPath)
	return nil
}

// formatPermissions convierte los permisos numéricos a formato de texto (por ejemplo, -rw-r--r--)
func formatPermissions(inode *structures.Inode) string {
	permStr := string(inode.I_perm[:])
	if len(permStr) != 3 {
		return "---------"
	}

	userPerm := int(permStr[0] - '0')
	groupPerm := int(permStr[1] - '0')
	otherPerm := int(permStr[2] - '0')

	result := "-"
	if inode.I_type[0] == '1' {
		result = "d" // Carpeta
	}

	// Permisos del usuario
	result += permToString(userPerm, true)
	// Permisos del grupo
	result += permToString(groupPerm, false)
	// Permisos de otros
	result += permToString(otherPerm, false)

	return result
}

// permToString convierte un número de permisos a formato de texto (por ejemplo, 7 -> rwx)
func permToString(perm int, isOwner bool) string {
	result := ""
	// Lectura
	if perm&4 != 0 {
		result += "r"
	} else {
		result += "-"
	}
	// Escritura
	if perm&2 != 0 {
		result += "w"
	} else {
		result += "-"
	}
	// Ejecución
	if perm&1 != 0 {
		result += "x"
	} else {
		result += "-"
	}
	return result
}

// getOwnerAndGroup obtiene el nombre del propietario y del grupo a partir de los IDs en el inodo
func getOwnerAndGroup(sb *structures.SuperBlock, diskPath string, inode *structures.Inode) (string, string, error) {
	// Leer el archivo users.txt para obtener los nombres de usuario y grupo
	usersInode := &structures.Inode{}
	err := usersInode.Deserialize(diskPath, int64(sb.S_inode_start+sb.S_inode_size)) // Inodo 1 es users.txt (creado por mkfs)
	if err != nil {
		return "", "", fmt.Errorf("error al cargar inodo de users.txt: %v", err)
	}

	content, err := readFileContent(sb, diskPath, usersInode)
	if err != nil {
		return "", "", fmt.Errorf("error al leer users.txt: %v", err)
	}

	// Parsear el contenido de users.txt
	lines := strings.Split(content, "\n")
	owner := "Desconocido"
	group := "Desconocido"

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) != 5 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		tipo := strings.TrimSpace(parts[1])
		groupName := strings.TrimSpace(parts[2]) // Campo "Grupo" en la línea de tipo U
		userName := strings.TrimSpace(parts[3])  // Campo "Usuario" en la línea de tipo U

		if tipo == "U" && fmt.Sprintf("%d", inode.I_uid) == id {
			owner = userName
			group = groupName // Obtener el grupo directamente de la línea del usuario
			break
		}
	}

	return owner, group, nil
}
