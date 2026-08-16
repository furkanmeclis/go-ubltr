<?xml version="1.0" encoding="UTF-8"?>
<xsl:stylesheet version="3.0"
    xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
    xmlns:inv="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
    xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
    exclude-result-prefixes="inv cbc">
  <xsl:output method="html" html-version="5.0" encoding="UTF-8"/>
  <xsl:template match="/">
    <html lang="tr">
      <head>
        <meta charset="UTF-8"/>
        <title>Özel stil</title>
      </head>
      <body>
        <h1>Özel XSLT</h1>
        <p>Fatura no: <xsl:value-of select="/inv:Invoice/cbc:ID"/></p>
      </body>
    </html>
  </xsl:template>
</xsl:stylesheet>
