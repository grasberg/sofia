# Test med specialtecken och Unicode

Detta testar hantering av olika tecken:

## Svenska tecken
Å Ä Ö å ä ö
É é È è Ê ê
German: ß ü ö ä

## Specialtecken
© ® ™ € £ ¥ § ¶ † ‡ • … — – -
Matematik: α β γ δ ε π σ μ ∞ ≠ ≤ ≥ √ ∫ ∑ ∏

## Emoji och symboler
👍 🚀 ⭐ 💡 ✅ ❌ 🔥 📈 🎯

## HTML-entiteter (ska inte escapeas)
<tag> & " '

## Långa rader
Detta är en mycket lång rad som borde brytas automatiskt i PDF:en men vi vill se hur pandoc och weasyprint hanterar långa textrader utan explicita radbrytningar. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.

## Tabeller (Markdown-format)

| Kolumn 1 | Kolumn 2 | Kolumn 3 |
|----------|----------|----------|
| Värde A  | Värde B  | Värde C  |
| Data 1   | Data 2   | Data 3   |
| **Fet**  | *Kursiv* | `Kod`    |

## Kod med syntax

```python
def hello_world():
    """En Python-funktion"""
    print("Hej världen!")
    return 42

# Kommentar med åäö
```

```javascript
// JavaScript-kod
function greet(name) {
    return `Hello, ${name}!`;
}
```

---

**Test slutförd**  
Önskar att PDF:en ser bra ut!