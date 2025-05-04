package commands

import (
	structures "backend/structures"
	utils "backend/utils"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type MKDISK struct {
	size int
	unit string
	fit  string
	path string
}

func ParseMkdisk(tokens []string) (string, error) {
	cmd := &MKDISK{}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas con (?i)
	re := regexp.MustCompile(`(?i)-size=\d+|(?i)-unit=[kKmM]|(?i)-fit=[bBfFwW]{2}|(?i)-path="[^"]+"|(?i)-path=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	// Expresión regular para capturar todos los parámetros, insensible a mayúsculas/minúsculas con (?i)
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-size": true,
		"-unit": true,
		"-fit":  true,
		"-path": true,
	}

	for _, param := range allParams {
		// Extraer solo el nombre del parámetro (sin el valor) y convertir a minúsculas
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		if !allowedParams[paramName] {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	// Procesar los parámetros válidos capturados por la regex
	for _, match := range matches {
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		// Convertir el nombre del parámetro a minúsculas para la comparación
		key, value := strings.ToLower(kv[0]), kv[1]

		// Remove quotes from value if present
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}
		switch key {
		case "-size":
			size, err := strconv.Atoi(value)
			if err != nil || size <= 0 {
				return "", fmt.Errorf("el tamaño debe ser un número entero positivo")
			}
			cmd.size = size
		case "-unit":
			// Convertir el valor a mayúsculas para mantener consistencia
			value = strings.ToUpper(value)
			if value != "K" && value != "M" {
				return "", fmt.Errorf("la unidad debe ser K o M")
			}
			cmd.unit = value
		case "-fit":
			// Convertir el valor a mayúsculas para mantener consistencia
			value = strings.ToUpper(value)
			if value != "BF" && value != "FF" && value != "WF" {
				return "", fmt.Errorf("el ajuste debe ser BF, FF o WF")
			}
			cmd.fit = value
		case "-path":
			if value == "" {
				return "", fmt.Errorf("el path no puede estar vacío")
			}
			cmd.path = value
		}
	}

	if cmd.size == 0 || cmd.path == "" {
		return "", fmt.Errorf("faltan parámetros requeridos: -size y -path")
	}
	if cmd.unit == "" {
		cmd.unit = "M"
	}
	if cmd.fit == "" {
		cmd.fit = "FF"
	}

	fmt.Println("DEBUG: Parseando MKDISK")
	fmt.Printf("  size=%d, unit=%s, fit=%s, path=%s\n", cmd.size, cmd.unit, cmd.fit, cmd.path)

	err := commandMkdisk(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MKDISK: Disco creado exitosamente\n-> Path: %s\n-> Tamaño: %d%s\n-> Fit: %s", cmd.path, cmd.size, cmd.unit, cmd.fit), nil
}

func commandMkdisk(mkdisk *MKDISK) error {
	sizeBytes, err := utils.ConvertToBytes(mkdisk.size, mkdisk.unit)
	if err != nil {
		fmt.Println("DEBUG: Error convirtiendo tamaño:", err)
		return err
	}
	fmt.Println("DEBUG: Tamaño en bytes:", sizeBytes)

	err = createDisk(mkdisk, sizeBytes)
	if err != nil {
		return err
	}

	err = createMBR(mkdisk, sizeBytes)
	if err != nil {
		return err
	}

	return nil
}

func createDisk(mkdisk *MKDISK, sizeBytes int) error {
	fmt.Println("DEBUG: Creando directorios para:", mkdisk.path)
	err := os.MkdirAll(filepath.Dir(mkdisk.path), os.ModePerm)
	if err != nil {
		fmt.Println("DEBUG: Error creando directorios:", err)
		return err
	}

	fmt.Println("DEBUG: Creando archivo en:", mkdisk.path)
	file, err := os.Create(mkdisk.path)
	if err != nil {
		fmt.Println("DEBUG: Error creando archivo:", err)
		return err
	}
	defer file.Close()

	buffer := make([]byte, 1024*1024)
	for sizeBytes > 0 {
		writeSize := len(buffer)
		if sizeBytes < writeSize {
			writeSize = sizeBytes
		}
		if _, err := file.Write(buffer[:writeSize]); err != nil {
			fmt.Println("DEBUG: Error escribiendo en archivo:", err)
			return err
		}
		sizeBytes -= writeSize
	}
	fmt.Println("DEBUG: Disco creado con éxito")
	return nil
}

func createMBR(mkdisk *MKDISK, sizeBytes int) error {
	var fitByte byte
	switch mkdisk.fit {
	case "FF":
		fitByte = 'F'
	case "BF":
		fitByte = 'B'
	case "WF":
		fitByte = 'W'
	default:
		return fmt.Errorf("tipo de ajuste inválido")
	}

	mbr := &structures.MBR{
		Mbr_size:           int32(sizeBytes),
		Mbr_creation_date:  float32(time.Now().Unix()),
		Mbr_disk_signature: rand.Int31(),
		Mbr_disk_fit:       [1]byte{fitByte},
		Mbr_partitions: [4]structures.Partition{
			{Part_start: -1, Part_correlative: -1},
			{Part_start: -1, Part_correlative: -1},
			{Part_start: -1, Part_correlative: -1},
			{Part_start: -1, Part_correlative: -1},
		},
	}

	fmt.Println("DEBUG: MBR inicializado")
	mbr.PrintMBR()
	fmt.Println("DEBUG: Particiones iniciales:")
	mbr.PrintPartitions()

	err := mbr.Serialize(mkdisk.path)
	if err != nil {
		fmt.Println("DEBUG: Error serializando MBR:", err)
		return err
	}
	fmt.Println("DEBUG: MBR serializado con éxito")
	return nil
}
