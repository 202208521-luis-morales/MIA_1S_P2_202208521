package reports

import (
	structures "backend/structures"
	utils "backend/utils"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ReportSB genera un reporte del superbloque y lo guarda en la ruta especificada
func ReportSB(sb *structures.SuperBlock, diskPath, outputPath string) error {
	// Crear las carpetas padre si no existen
	err := utils.CreateParentDirs(outputPath)
	if err != nil {
		return err
	}

	// Obtener el nombre base del archivo sin la extensión
	dotFileName, outputImage := utils.GetFileNames(outputPath)

	// Definir el contenido DOT con una tabla
	diskName := filepath.Base(diskPath)
	dotContent := fmt.Sprintf(`digraph G {
        node [shape=plaintext]
        tabla [label=<
            <table border="0" cellborder="1" cellspacing="0">
                <tr><td colspan="2">Reporte de SUPERBLOQUE</td></tr>
                <tr><td colspan="2">#SuperBloque en disco %s</td></tr>
                <tr><td>sb_inodos_count</td><td>%d</td></tr>
                <tr><td>sb_bloques_count</td><td>%d</td></tr>
                <tr><td>sb_inodos_free</td><td>%d</td></tr>
                <tr><td>sb_bloques_free</td><td>%d</td></tr>
                <tr><td>sb_date_ultimo_montaje</td><td>%s</td></tr>
                <tr><td>sb_date_ultimo_desmontaje</td><td>%s</td></tr>
                <tr><td>sb_montajes_count</td><td>%d</td></tr>
                <tr><td>sb_magic_num</td><td>%d</td></tr>
                <tr><td>sb_size_struct_inodo</td><td>%d</td></tr>
                <tr><td>sb_size_struct_bloque</td><td>%d</td></tr>
                <tr><td>sb_first_free_bit_inodos</td><td>%d</td></tr>
                <tr><td>sb_first_free_bit_bloques</td><td>%d</td></tr>
                <tr><td>sb_ap_bitmap_inodos</td><td>%d</td></tr>
                <tr><td>sb_ap_bitmap_bloques</td><td>%d</td></tr>
                <tr><td>sb_ap_inodos</td><td>%d</td></tr>
                <tr><td>sb_ap_bloques</td><td>%d</td></tr>
            `, diskName,
		sb.S_inodes_count, sb.S_blocks_count, sb.S_free_inodes_count, sb.S_free_blocks_count,
		time.Unix(int64(sb.S_mtime), 0).Format("2006-01-02 15:04"),
		time.Unix(int64(sb.S_umtime), 0).Format("2006-01-02 15:04"),
		sb.S_mnt_count, sb.S_magic, sb.S_inode_size, sb.S_block_size,
		sb.S_first_ino, sb.S_first_blo, sb.S_bm_inode_start, sb.S_bm_block_start,
		sb.S_inode_start, sb.S_block_start)

	// Cerrar la tabla y el contenido DOT
	dotContent += "</table>>] }"

	// Guardar el contenido DOT en un archivo
	file, err := os.Create(dotFileName)
	if err != nil {
		return fmt.Errorf("error al crear el archivo DOT: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString(dotContent)
	if err != nil {
		return fmt.Errorf("error al escribir en el archivo DOT: %v", err)
	}

	// Ejecutar el comando Graphviz para generar la imagen
	cmd := exec.Command("dot", "-Tjpg", dotFileName, "-o", outputImage)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error al ejecutar el comando Graphviz: %v", err)
	}

	fmt.Println("DEBUG: Imagen del reporte de superbloque generada:", outputImage)
	return nil
}
