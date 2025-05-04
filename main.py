def hex_to_binary(hex_string):
    # Eliminar espacios y convertir a una lista de bytes en hexadecimal
    hex_values = hex_string.replace("\n", " ").split()
    
    # Filtrar los encabezados de columna (como 00 01 02... en la primera línea)
    # y cualquier otro valor no hexadecimal
    filtered_hex = []
    for i, val in enumerate(hex_values):
        # Verificar si el valor es un byte hexadecimal válido (00-FF)
        if len(val) == 2 and all(c in '0123456789ABCDEFabcdef' for c in val):
            filtered_hex.append(val)
    
    # Convertir cada byte hexadecimal a su representación binaria
    binary_list = []
    for hex_val in filtered_hex:
        # Convertir hex a int, luego a binario, y eliminar el prefijo '0b'
        binary = bin(int(hex_val, 16))[2:].zfill(8)
        binary_list.append(binary)
    
    # Unir todos los binarios en una sola cadena sin espacios
    return ''.join(binary_list)

# El texto hexadecimal del volcado (copia y pega aquí el texto completo)
hex_dump = """
00 01 02 03 04 05 06 07 08 09 0A 0B 0C 0D 0E 0F
00 00 A0 00 54 DB CF 4E 7E 8A 72 6D 46 31 50 57
99 00 00 00 00 00 20 00 50 61 72 74 31 00 00 00
00 00 00 00 00 00 00 00 01 00 00 00 32 31 31 41
00 00 00 FF FF FF FF 00 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 FF FF FF FF 00
00 00 00 00 00 00 FF FF FF FF 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 FF FF
FF FF 00 00 00 00 00 00 00 FF FF FF FF 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
00 FF FF FF FF 00 00 00 00 00 00 00 00 02 00 00
00 02 00 00 00 D6 1C 00 00 86 56 00 00 54 DB CF
4E 54 DB CF 4E 00 00 00 00 00 00 00 00 58 00 00
00 40 00 00 00 02 00 00 00 FD 5E 0A 00 DD 00 00
00 B5 1D 00 00 3D 74 00 00 7D 5E 0A 00 31 31 30
"""

# Ejecutar la conversión
binary_result = hex_to_binary(hex_dump)

# Imprimir las primeras 200 posiciones del resultado binario (el resultado completo podría ser muy largo)
print("Primeros 200 bits del resultado binario:")
print(binary_result[:200])

# Imprimir la longitud total
print(f"\nLongitud total del binario: {len(binary_result)} bits")

# Guardar el resultado completo en un archivo
with open("binary_output.txt", "w") as f:
    f.write(binary_result)

print("\nEl resultado completo se ha guardado en 'binary_output.txt'")

# Si deseas ver el resultado completo, descomenta la siguiente línea
# print("Resultado binario completo:")
# print(binary_result)