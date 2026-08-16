<?xml version="1.0" encoding="UTF-8"?>
<xsl:stylesheet version="3.0"
    xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
    xmlns:inv="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
    xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
    xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
    exclude-result-prefixes="inv cac cbc">

  <xsl:output method="html" html-version="5.0" encoding="UTF-8" indent="yes"/>

  <xsl:template match="/">
    <html lang="tr">
      <head>
        <meta charset="UTF-8"/>
        <title>
          Fatura
          <xsl:value-of select="/inv:Invoice/cbc:ID"/>
        </title>
        <style type="text/css">
          @page { size: A4; margin: 16mm; }
          body { font-family: "Noto Sans", "DejaVu Sans", sans-serif; color: #111; }
          h1 { font-size: 20px; }
          table { width: 100%; border-collapse: collapse; }
          th, td { border: 1px solid #ccc; padding: 6px; text-align: left; }
          thead { display: table-header-group; }
          tr { page-break-inside: avoid; }
        </style>
      </head>
      <body>
        <xsl:apply-templates select="/inv:Invoice"/>
      </body>
    </html>
  </xsl:template>

  <xsl:template match="inv:Invoice">
    <h1>Fatura <xsl:value-of select="cbc:ID"/></h1>
    <p class="profile"><xsl:value-of select="cbc:ProfileID"/></p>
    <p class="issue-date"><xsl:value-of select="cbc:IssueDate"/></p>
    <section class="supplier">
      <h2>Satıcı</h2>
      <p class="party-name">
        <xsl:value-of select="cac:AccountingSupplierParty/cac:Party/cac:PartyName/cbc:Name"/>
      </p>
      <p class="city">
        <xsl:value-of select="cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cbc:CityName"/>
      </p>
      <p class="country">
        <xsl:value-of select="cac:AccountingSupplierParty/cac:Party/cac:PostalAddress/cac:Country/cbc:Name"/>
      </p>
    </section>
    <table>
      <thead>
        <tr>
          <th>Kalem</th>
          <th>Tutar</th>
        </tr>
      </thead>
      <tbody>
        <xsl:for-each select="cac:InvoiceLine">
          <tr>
            <td><xsl:value-of select="cac:Item/cbc:Name"/></td>
            <td><xsl:value-of select="cbc:LineExtensionAmount"/></td>
          </tr>
        </xsl:for-each>
      </tbody>
    </table>
    <p class="payable">
      <xsl:value-of select="cac:LegalMonetaryTotal/cbc:PayableAmount"/>
    </p>
    <p class="glyphs">ı İş Ş ğ Ğ ü Ü ö Ö ç Ç</p>
  </xsl:template>

</xsl:stylesheet>
