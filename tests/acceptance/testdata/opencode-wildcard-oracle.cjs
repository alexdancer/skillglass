function match(str, pattern) {
  if (str) str = str.replaceAll("\\", "/")
  if (pattern) pattern = pattern.replaceAll("\\", "/")
  let escaped = pattern
    .replace(/[.+^${}()|[\]\\]/g, "\\$&")
    .replace(/\*/g, ".*")
    .replace(/\?/g, ".")
  if (escaped.endsWith(" .*")) {
    escaped = escaped.slice(0, -3) + "( .*)?"
  }
  const flags = process.platform === "win32" ? "si" : "s"
  return new RegExp("^" + escaped + "$", flags).test(str)
}
const pairs = [
  ["🔍", "?"], ["🔍", "??"], ["🔍", "🔍"], ["🔍", "*"],
  ["🔍x", "??x"], ["🔍x", "?x"], ["🔍x", "🔍?"], ["🔍x", "*?"],
  ["x🔍", "x??"], ["x🔍", "x?"], ["🔍🔍", "????"], ["🔍🔍", "??"],
  ["é", "?"], ["é", "??"], ["x", "?"], ["xy", "??"], ["xy", "?"],
  ["a.b+[c](d)$^|", "a.b+[c](d)$^|"], ["a.b", "a?b"], ["Ab", "ab"],
  ["slash\\name", "slash/name"], ["slash/name", "slash\\name"],
  ["command", "command *"], ["command args", "command *"], ["commandx", "command *"],
  ["line\nbreak", "line?break"], ["line\nbreak", "line*break"],
  ["x\n", "x"], ["x\r\n", "x"], ["x\u2028", "x"], ["x\u2029", "x"],
  ["𐀀", "?"], ["𐀀", "??"], ["\ue000", "?"], ["\ue000", "🔍"],
  ["skill", "sk?ll"], ["skill", "s*"], ["skill", "sk??ll"],
  ["🔍", "\ud83d?"], ["🔍", "?\udd0d"], ["🔍", "\ud83d\u0061"]
]
process.stdout.write(JSON.stringify(pairs.map(([value, pattern]) => ({value, pattern, pattern_json: JSON.stringify(pattern), scalar_pattern: pattern.isWellFormed(), matched: match(value, pattern)})), null, 2) + "\n")
