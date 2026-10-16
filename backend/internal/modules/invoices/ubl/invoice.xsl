<?xml version="1.0" encoding="UTF-8"?>
<!--
  Rentopia invoice view: UBL-TR Invoice (e-Fatura / e-Arşiv) → HTML.
  Used for the panel preview, the customer portal and the PDF (headless
  Chromium). No scripts, no external loads (the PDF print blocks them).
-->
<xsl:stylesheet version="3.0"
    xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
    xmlns:inv="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
    xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
    xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
    exclude-result-prefixes="inv cac cbc">

  <xsl:output method="html" html-version="5.0" encoding="UTF-8" indent="no"/>
  <xsl:decimal-format name="tr" decimal-separator="," grouping-separator="."/>

  <xsl:template name="amount">
    <xsl:param name="v"/>
    <xsl:param name="cur" select="'TRY'"/>
    <xsl:value-of select="format-number(number($v), '#.##0,00', 'tr')"/>
    <xsl:text> </xsl:text>
    <xsl:choose>
      <xsl:when test="$cur = 'TRY'">TL</xsl:when>
      <xsl:otherwise><xsl:value-of select="$cur"/></xsl:otherwise>
    </xsl:choose>
  </xsl:template>

  <xsl:template name="party">
    <xsl:param name="p"/>
    <div class="party-name">
      <xsl:choose>
        <xsl:when test="normalize-space($p/cac:PartyName/cbc:Name) != ''">
          <xsl:value-of select="$p/cac:PartyName/cbc:Name"/>
        </xsl:when>
        <xsl:otherwise>
          <xsl:value-of select="$p/cac:Person/cbc:FirstName"/>
          <xsl:text> </xsl:text>
          <xsl:value-of select="$p/cac:Person/cbc:FamilyName"/>
        </xsl:otherwise>
      </xsl:choose>
    </div>
    <div>
      <xsl:value-of select="$p/cac:PostalAddress/cbc:StreetName"/>
    </div>
    <div>
      <xsl:value-of select="$p/cac:PostalAddress/cbc:CitySubdivisionName"/>
      <xsl:text> / </xsl:text>
      <xsl:value-of select="$p/cac:PostalAddress/cbc:CityName"/>
      <xsl:if test="normalize-space($p/cac:PostalAddress/cbc:PostalZone) != ''">
        <xsl:text> </xsl:text>
        <xsl:value-of select="$p/cac:PostalAddress/cbc:PostalZone"/>
      </xsl:if>
    </div>
    <xsl:if test="normalize-space($p/cac:PartyTaxScheme/cac:TaxScheme/cbc:Name) != ''">
      <div>Vergi dairesi: <xsl:value-of select="$p/cac:PartyTaxScheme/cac:TaxScheme/cbc:Name"/></div>
    </xsl:if>
    <xsl:for-each select="$p/cac:PartyIdentification/cbc:ID">
      <div><xsl:value-of select="@schemeID"/>: <xsl:value-of select="."/></div>
    </xsl:for-each>
    <xsl:if test="normalize-space($p/cac:Contact/cbc:ElectronicMail) != ''">
      <div>E-posta: <xsl:value-of select="$p/cac:Contact/cbc:ElectronicMail"/></div>
    </xsl:if>
  </xsl:template>

  <xsl:template match="/">
    <html lang="tr">
      <head>
        <meta charset="UTF-8"/>
        <meta name="viewport" content="width=device-width, initial-scale=1"/>
        <title>Fatura <xsl:value-of select="/inv:Invoice/cbc:ID"/></title>
        <style type="text/css">
          @page { size: A4; margin: 14mm; }
          * { box-sizing: border-box; }
          body { margin: 0; padding: 16px; font-family: "Noto Sans", "DejaVu Sans", Arial, sans-serif; font-size: 12px; color: #18181b; background: #fff; }
          .doc { max-width: 820px; margin: 0 auto; }
          .head { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 16px; border-bottom: 2px solid #18181b; padding-bottom: 12px; }
          .title { font-size: 20px; font-weight: 700; letter-spacing: .02em; }
          .subtitle { color: #52525b; margin-top: 2px; }
          .badge { display: inline-block; border: 1px solid #18181b; border-radius: 4px; padding: 2px 8px; font-weight: 600; font-size: 11px; }
          .parties { display: flex; flex-wrap: wrap; gap: 16px; margin: 16px 0; }
          .party { flex: 1 1 260px; border: 1px solid #e4e4e7; border-radius: 6px; padding: 10px 12px; line-height: 1.5; }
          .party h2, .meta h2 { margin: 0 0 6px; font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: #71717a; }
          .party-name { font-weight: 600; font-size: 13px; }
          .meta { border: 1px solid #e4e4e7; border-radius: 6px; padding: 10px 12px; margin-bottom: 16px; }
          .meta table td { padding: 2px 12px 2px 0; border: 0; }
          .meta table td:first-child { color: #71717a; white-space: nowrap; }
          .ettn { font-family: "DejaVu Sans Mono", monospace; word-break: break-all; }
          table.lines { width: 100%; border-collapse: collapse; }
          table.lines th { background: #f4f4f5; text-align: left; font-weight: 600; padding: 6px 8px; border-bottom: 1px solid #d4d4d8; font-size: 11px; }
          table.lines td { padding: 6px 8px; border-bottom: 1px solid #e4e4e7; vertical-align: top; }
          table.lines .num { text-align: right; white-space: nowrap; }
          thead { display: table-header-group; }
          tr { page-break-inside: avoid; }
          .totals { margin-left: auto; margin-top: 12px; width: 100%; max-width: 360px; border-collapse: collapse; }
          .totals td { padding: 4px 8px; }
          .totals td:last-child { text-align: right; white-space: nowrap; }
          .totals tr.grand td { border-top: 2px solid #18181b; font-weight: 700; font-size: 13px; }
          .notes { margin-top: 16px; color: #3f3f46; line-height: 1.5; }
          .return { margin: 0 0 16px; padding: 8px 12px; border-left: 3px solid #b45309; background: #fffbeb; }
          @media (max-width: 520px) { body { padding: 8px; } table.lines .hide-sm { display: none; } }
        </style>
      </head>
      <body>
        <xsl:apply-templates select="/inv:Invoice"/>
      </body>
    </html>
  </xsl:template>

  <xsl:template match="inv:Invoice">
    <xsl:variable name="cur" select="string(cbc:DocumentCurrencyCode)"/>
    <div class="doc">
      <div class="head">
        <div>
          <div class="title">
            <xsl:choose>
              <xsl:when test="cbc:ProfileID = 'EARSIVFATURA'">e-Arşiv Fatura</xsl:when>
              <xsl:otherwise>e-Fatura</xsl:otherwise>
            </xsl:choose>
          </div>
          <div class="subtitle">
            <xsl:choose>
              <xsl:when test="cbc:InvoiceTypeCode = 'IADE'">İade faturası</xsl:when>
              <xsl:otherwise>Satış faturası</xsl:otherwise>
            </xsl:choose>
          </div>
        </div>
        <div><span class="badge"><xsl:value-of select="cbc:ProfileID"/></span></div>
      </div>

      <div class="parties">
        <div class="party">
          <h2>Satıcı</h2>
          <xsl:call-template name="party">
            <xsl:with-param name="p" select="cac:AccountingSupplierParty/cac:Party"/>
          </xsl:call-template>
        </div>
        <div class="party">
          <h2>Alıcı</h2>
          <xsl:call-template name="party">
            <xsl:with-param name="p" select="cac:AccountingCustomerParty/cac:Party"/>
          </xsl:call-template>
        </div>
      </div>

      <div class="meta">
        <h2>Fatura bilgileri</h2>
        <table>
          <tr><td>Fatura no</td><td><xsl:value-of select="cbc:ID"/></td></tr>
          <tr><td>Düzenleme tarihi</td><td><xsl:value-of select="cbc:IssueDate"/><xsl:text> </xsl:text><xsl:value-of select="cbc:IssueTime"/></td></tr>
          <tr><td>Senaryo</td><td><xsl:value-of select="cbc:ProfileID"/></td></tr>
          <tr><td>Fatura tipi</td><td><xsl:value-of select="cbc:InvoiceTypeCode"/></td></tr>
          <tr><td>ETTN</td><td class="ettn"><xsl:value-of select="cbc:UUID"/></td></tr>
        </table>
      </div>

      <xsl:for-each select="cac:BillingReference/cac:InvoiceDocumentReference">
        <div class="return">
          İade edilen fatura: <strong><xsl:value-of select="cbc:ID"/></strong>
          (<xsl:value-of select="cbc:IssueDate"/>)
        </div>
      </xsl:for-each>

      <table class="lines">
        <thead>
          <tr>
            <th>#</th>
            <th>Açıklama</th>
            <th class="num">Miktar</th>
            <th class="num hide-sm">Birim fiyat</th>
            <th class="num">KDV %</th>
            <th class="num hide-sm">KDV</th>
            <th class="num">Tutar</th>
          </tr>
        </thead>
        <tbody>
          <xsl:for-each select="cac:InvoiceLine">
            <tr>
              <td><xsl:value-of select="cbc:ID"/></td>
              <td>
                <xsl:value-of select="cac:Item/cbc:Name"/>
                <xsl:for-each select="cac:TaxTotal/cac:TaxSubtotal/cac:TaxCategory[normalize-space(cbc:TaxExemptionReasonCode) != '']">
                  <div class="subtitle">İstisna <xsl:value-of select="cbc:TaxExemptionReasonCode"/>: <xsl:value-of select="cbc:TaxExemptionReason"/></div>
                </xsl:for-each>
              </td>
              <td class="num">
                <xsl:value-of select="number(cbc:InvoicedQuantity)"/>
                <xsl:text> </xsl:text>
                <xsl:choose>
                  <xsl:when test="cbc:InvoicedQuantity/@unitCode = 'DAY'">gün</xsl:when>
                  <xsl:otherwise>adet</xsl:otherwise>
                </xsl:choose>
              </td>
              <td class="num hide-sm">
                <xsl:value-of select="format-number(number(cac:Price/cbc:PriceAmount), '#.##0,00##', 'tr')"/>
              </td>
              <td class="num"><xsl:value-of select="number(cac:TaxTotal/cac:TaxSubtotal/cbc:Percent)"/></td>
              <td class="num hide-sm">
                <xsl:call-template name="amount">
                  <xsl:with-param name="v" select="cac:TaxTotal/cbc:TaxAmount"/>
                  <xsl:with-param name="cur" select="$cur"/>
                </xsl:call-template>
              </td>
              <td class="num">
                <xsl:call-template name="amount">
                  <xsl:with-param name="v" select="cbc:LineExtensionAmount"/>
                  <xsl:with-param name="cur" select="$cur"/>
                </xsl:call-template>
              </td>
            </tr>
          </xsl:for-each>
        </tbody>
      </table>

      <table class="totals">
        <tr>
          <td>Mal / hizmet toplamı</td>
          <td>
            <xsl:call-template name="amount">
              <xsl:with-param name="v" select="cac:LegalMonetaryTotal/cbc:LineExtensionAmount"/>
              <xsl:with-param name="cur" select="$cur"/>
            </xsl:call-template>
          </td>
        </tr>
        <xsl:for-each select="cac:TaxTotal/cac:TaxSubtotal">
          <tr>
            <td>Hesaplanan KDV (%<xsl:value-of select="number(cbc:Percent)"/>)</td>
            <td>
              <xsl:call-template name="amount">
                <xsl:with-param name="v" select="cbc:TaxAmount"/>
                <xsl:with-param name="cur" select="$cur"/>
              </xsl:call-template>
            </td>
          </tr>
        </xsl:for-each>
        <tr>
          <td>Vergiler dahil toplam</td>
          <td>
            <xsl:call-template name="amount">
              <xsl:with-param name="v" select="cac:LegalMonetaryTotal/cbc:TaxInclusiveAmount"/>
              <xsl:with-param name="cur" select="$cur"/>
            </xsl:call-template>
          </td>
        </tr>
        <tr class="grand">
          <td>Ödenecek tutar</td>
          <td>
            <xsl:call-template name="amount">
              <xsl:with-param name="v" select="cac:LegalMonetaryTotal/cbc:PayableAmount"/>
              <xsl:with-param name="cur" select="$cur"/>
            </xsl:call-template>
          </td>
        </tr>
      </table>

      <xsl:if test="cbc:Note">
        <div class="notes">
          <xsl:for-each select="cbc:Note">
            <div><xsl:value-of select="."/></div>
          </xsl:for-each>
        </div>
      </xsl:if>
    </div>
  </xsl:template>

</xsl:stylesheet>
