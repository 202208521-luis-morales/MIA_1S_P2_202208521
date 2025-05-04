package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// LOGIN estructura que representa el comando login con sus parámetros
type LOGIN struct {
	user string // Nombre del usuario
	pass string // Contraseña
	id   string // ID de la partición
}

// ParseLogin parsea el comando login y autentica al usuario
func ParseLogin(tokens []string) (string, error) {
	cmd := &LOGIN{}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-user=[^\s]+|(?i)-pass=[^\s]+|(?i)-id=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-user": true,
		"-pass": true,
		"-id":   true,
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
		switch key {
		case "-user":
			if value == "" {
				return "", errors.New("el usuario no puede estar vacío")
			}
			cmd.user = value
		case "-pass":
			if value == "" {
				return "", errors.New("la contraseña no puede estar vacía")
			}
			cmd.pass = value
		case "-id":
			if value == "" {
				return "", errors.New("el ID no puede estar vacío")
			}
			cmd.id = value
		}
	}
	if cmd.user == "" || cmd.pass == "" || cmd.id == "" {
		return "", errors.New("faltan parámetros requeridos: -user, -pass, -id")
	}

	err := commandLogin(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("LOGIN: Sesión iniciada exitosamente\n-> Usuario: %s\n-> ID: %s", cmd.user, cmd.id), nil
}

// commandLogin autentica al usuario y establece la sesión con debug extremo
func commandLogin(login *LOGIN) error {
	fmt.Println("\nDEBUG: Iniciando commandLogin - User:", login.user, "ID:", login.id)

	if stores.ActiveSession != nil {
		return errors.New("ya hay una sesión activa, cierre la sesión actual primero")
	}

	sb, partition, path, err := stores.GetMountedPartitionSuperblock(login.id)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}
	fmt.Println("DEBUG: Partición encontrada - Path:", path, "Part_start:", partition.Part_start)
	fmt.Println("DEBUG: SuperBlock:")
	sb.Print()

	fmt.Println("DEBUG: Cargando inodo raíz desde S_inode_start:", sb.S_inode_start)
	rootInode := &structures.Inode{}
	err = rootInode.Deserialize(path, int64(sb.S_inode_start))
	if err != nil {
		return fmt.Errorf("error al cargar inodo raíz: %v", err)
	}
	fmt.Println("DEBUG: Inodo raíz cargado - I_block[0]:", rootInode.I_block[0], "I_type:", string(rootInode.I_type[:]), "I_perm:", string(rootInode.I_perm[:]))

	fmt.Println("DEBUG: Cargando bloque raíz desde I_block[0]:", rootInode.I_block[0])
	rootBlock := &structures.FolderBlock{}
	err = rootBlock.Deserialize(path, int64(rootInode.I_block[0]))
	if err != nil {
		return fmt.Errorf("error al cargar bloque raíz: %v", err)
	}
	fmt.Println("DEBUG: Bloque raíz cargado. Buscando users.txt...")
	for i, content := range rootBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", i, name, content.B_inodo)
		if name == "users.txt" {
			usersInodeIndex := content.B_inodo
			fmt.Println("DEBUG: users.txt encontrado en inodo:", usersInodeIndex)

			fmt.Println("DEBUG: Cargando inodo de users.txt desde:", sb.S_inode_start+usersInodeIndex*sb.S_inode_size)
			usersInode := &structures.Inode{}
			err = usersInode.Deserialize(path, int64(sb.S_inode_start+usersInodeIndex*sb.S_inode_size))
			if err != nil {
				return fmt.Errorf("error al cargar inodo de users.txt: %v", err)
			}
			fmt.Println("DEBUG: Inodo de users.txt cargado - I_block[0]:", usersInode.I_block[0], "I_size:", usersInode.I_size)

			fmt.Println("DEBUG: Leyendo contenido de users.txt desde bloque:", usersInode.I_block[0])
			content, err := readFileContent(sb, path, usersInode)
			if err != nil {
				return fmt.Errorf("error al leer users.txt: %v", err)
			}
			fmt.Println("DEBUG: Después de leer users.txt")
			fmt.Printf("DEBUG: Contenido de users.txt: %q\n", content)

			lines := strings.Split(content, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				fields := strings.Split(line, ",")
				if len(fields) != 5 {
					fmt.Println("DEBUG: Línea ignorada por formato inválido:", line)
					continue
				}
				for i := range fields {
					fields[i] = strings.TrimSpace(fields[i])
				}
				fmt.Println("DEBUG: Línea parseada:", fields)
				if fields[1] == "U" && fields[2] == login.user && fields[4] == login.pass {
					userID, _ := parseInt32Login(fields[0])
					var groupID int32
					for _, gLine := range lines {
						gFields := strings.Split(strings.TrimSpace(gLine), ",")
						if len(gFields) != 5 {
							continue
						}
						for i := range gFields {
							gFields[i] = strings.TrimSpace(gFields[i])
						}
						if gFields[1] == "G" && gFields[2] == fields[3] {
							groupID, _ = parseInt32Login(gFields[0])
							break
						}
					}
					fmt.Println("DEBUG: Usuario autenticado - UserID:", userID, "GroupID:", groupID)
					fmt.Println("DEBUG: Antes de establecer ActiveSession")
					stores.ActiveSession = &stores.Session{
						Username:    login.user,
						UserID:      userID,
						GroupID:     groupID,
						PartitionID: login.id,
					}
					fmt.Println("DEBUG: Después de establecer ActiveSession")
					return nil
				}
			}
			return fmt.Errorf("el usuario %s no existe o la contraseña es incorrecta", login.user)
		}
	}

	return errors.New("el archivo users.txt no existe en la raíz")
}

func parseInt32Login(s string) (int32, error) {
	var i int32
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}
