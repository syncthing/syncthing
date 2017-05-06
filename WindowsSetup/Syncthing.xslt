<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform"   xmlns:msxsl="urn:schemas-microsoft-com:xslt"   exclude-result-prefixes="msxsl"   xmlns:wix="http://schemas.microsoft.com/wix/2006/wi"   xmlns:my="my:my"  xmlns:util="http://schemas.microsoft.com/wix/UtilExtension">
  <!-- http://www.chriskonces.com/using-xslt-with-heat-exe-wix-to-create-windows-service-installs/ -->
  <xsl:output method="xml" indent="yes" />
  <xsl:strip-space elements="*" />
  <xsl:template match="@*|node()">
    <xsl:copy>
      <xsl:apply-templates select="@*|node()" />
    </xsl:copy>
  </xsl:template>
  <xsl:template match="wix:Component[wix:File[@Source='$(var.MySource)\SyncthingServiceWrapper.exe']]">
    <xsl:copy>
      <xsl:apply-templates select="node() | @*" />
      <!--<util:User Id="UpdateUserLogonAsService" UpdateIfExists="yes" CreateUser="no" Name="[SERVICECREDENTIALS_USERLOGIN]"
            LogonAsService="yes" />-->
      <wix:ServiceInstall Id="MyServiceInstall" DisplayName="Syncthing Service" Description="Syncthingservice created by installer" Name="Syncthing Service" ErrorControl="ignore" Start="auto" Type="ownProcess" Vital="yes" Interactive="no" Account="LocalSystem" />
      <wix:ServiceControl Id="MyServiceControl" Name="Syncthing Service" Start="install" Stop="both" Remove="uninstall" Wait="yes" />
      <!--<util:User Id="user" CreateUser="no" Name ="[SERVICECREDENTIALS_USERLOGIN]" Password="[SERVICECREDENTIALS_PASSWORD]" LogonAsService="yes" />-->
    </xsl:copy>
  </xsl:template>
</xsl:stylesheet>