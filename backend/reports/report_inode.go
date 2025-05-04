package reports

import (
	structures "backend/structures"
	utils "backend/utils"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// ReportInode genera un reporte de los inodos utilizados con todos sus atributos y lo guarda en la ruta especificada
func ReportInode(superblock *structures.SuperBlock, diskPath string, path string) error {
	// Crear las carpetas padre si no existen
	err := utils.CreateParentDirs(path)
	if err != nil {
		return err
	}

	// Obtener el nombre base del archivo sin la extensión
	dotFileName, outputImage := utils.GetFileNames(path)

	// Iniciar el contenido DOT con rankdir=LR para disposición horizontal
	dotContent := `digraph G {
        rankdir=LR;
        node [shape=plaintext]
    `

	// Lista para almacenar los índices de los inodos utilizados
	usedInodes := []int32{}

	// Iterar sobre cada inodo para identificar los utilizados
	for i := int32(0); i < superblock.S_inodes_count; i++ {
		inode := &structures.Inode{}
		// Deserializar el inodo
		err := inode.Deserialize(diskPath, int64(superblock.S_inode_start+(i*superblock.S_inode_size)))
		if err != nil {
			return err
		}

		// Verificar si el inodo está utilizado
		// Un inodo está utilizado si tiene un tipo válido ('0' para carpeta, '1' para archivo)
		if inode.I_type[0] == '0' || inode.I_type[0] == '1' {
			usedInodes = append(usedInodes, i)
		}
	}

	// Si no hay inodos utilizados, generar un reporte vacío
	if len(usedInodes) == 0 {
		dotContent += `label="No hay inodos utilizados";`
	} else {
		// Generar el contenido DOT para cada inodo utilizado
		for idx, i := range usedInodes {
			inode := &structures.Inode{}
			err := inode.Deserialize(diskPath, int64(superblock.S_inode_start+(i*superblock.S_inode_size)))
			if err != nil {
				return err
			}

			// Convertir tiempos a string con formato DD/MM/YYYY HH:MM
			atime := time.Unix(int64(inode.I_atime), 0).Format("02/01/2006 15:04")
			ctime := time.Unix(int64(inode.I_ctime), 0).Format("02/01/2006 15:04")
			mtime := time.Unix(int64(inode.I_mtime), 0).Format("02/01/2006 15:04")

			// Definir el contenido DOT para el inodo actual
			dotContent += fmt.Sprintf(`inode%d [label=<
                <table border="0" cellborder="1" cellspacing="0">
                    <tr><td colspan="2"> Inodo %d </td></tr>
                    <tr><td>i_uid</td><td>%d</td></tr>
                    <tr><td>i_gid</td><td>%d</td></tr>
                    <tr><td>i_size</td><td>%d</td></tr>
                    <tr><td>i_atime</td><td>%s</td></tr>
                    <tr><td>i_ctime</td><td>%s</td></tr>
                    <tr><td>i_mtime</td><td>%s</td></tr>
                    <tr><td>i_type</td><td>%c</td></tr>
                    <tr><td>i_perm</td><td>%s</td></tr>
                    <tr><td colspan="2">Bloques Directos</td></tr>
                `, i, i, inode.I_uid, inode.I_gid, inode.I_size, atime, ctime, mtime, rune(inode.I_type[0]), string(inode.I_perm[:]))

			// Agregar los bloques directos (índices 0 a 11)
			for j := 0; j < 12; j++ {
				dotContent += fmt.Sprintf("<tr><td>i_block_%d</td><td>%d</td></tr>", j+1, inode.I_block[j])
			}

			// Agregar los bloques indirectos
			dotContent += fmt.Sprintf(`
                    <tr><td colspan="2">Bloque Indirecto</td></tr>
                    <tr><td>i_block_13</td><td>%d</td></tr>
                    <tr><td colspan="2">Bloque Indirecto Doble</td></tr>
                    <tr><td>i_block_14</td><td>%d</td></tr>
                    <tr><td colspan="2">Bloque Indirecto Triple</td></tr>
                    <tr><td>i_block_15</td><td>%d</td></tr>
                </table>>];
            `, inode.I_block[12], inode.I_block[13], inode.I_block[14])

			// Agregar enlace al siguiente inodo utilizado si no es el último
			if idx < len(usedInodes)-1 {
				dotContent += fmt.Sprintf("inode%d -> inode%d;\n", i, usedInodes[idx+1])
			}
		}
	}

	// Cerrar el contenido DOT
	dotContent += "}"

	// Crear el archivo DOT
	dotFile, err := os.Create(dotFileName)
	if err != nil {
		return err
	}
	defer dotFile.Close()

	// Escribir el contenido DOT en el archivo
	_, err = dotFile.WriteString(dotContent)
	if err != nil {
		return err
	}

	// Generar la imagen con Graphviz
	cmd := exec.Command("dot", "-Tpng", dotFileName, "-o", outputImage)
	err = cmd.Run()
	if err != nil {
		return err
	}

	fmt.Println("Imagen de los inodos generada:", outputImage)
	return nil
}
