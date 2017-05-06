using Microsoft.Deployment.WindowsInstaller;
using Microsoft.VisualStudio.TestTools.UnitTesting;
using Microsoft.Win32;
using System;
using System.Diagnostics;
using System.IO;
using System.Security.Principal;
using System.ServiceProcess;

namespace UnitTestSyncthingServiceWrapper
{
    [TestClass]
    public class UnitTest1
    {
        private string msiFilename;

        [TestInitialize]
        public void PrepareTest()
        {
            Assert.IsTrue(IsUserAdministrator(), "Tests need Adminrights");
            msiFilename = Path.Combine(GetCurrentExecutingDirectory(), "Syncthing.msi");
        }

        /// <summary>
        /// Gets the current executing directory.
        /// </summary>
        /// <returns></returns>
        private static string GetCurrentExecutingDirectory()
        {
            return Path.GetDirectoryName(new System.Uri(System.Reflection.Assembly.GetExecutingAssembly().CodeBase).LocalPath);
        }

        [TestMethod]
        public void ShouldstopSyncthingService()
        {
            if (!isSyncthingInstalled())
            {
                ShouldInstallSyncthingService();
            }
            var startSyncthingWrapper = new SyncthingServiceWrapper.SyncthingService();
            ServiceController service = new ServiceController(startSyncthingWrapper.ServiceName);
            if (service.Status == ServiceControllerStatus.Stopped)
            {
                service.Start();
            }
            service.WaitForStatus(ServiceControllerStatus.Running, new TimeSpan(0, 0, 10));
            service.Stop();
            service.WaitForStatus(ServiceControllerStatus.Stopped, new TimeSpan(0, 0, 10));
            Assert.IsTrue(service.Status == ServiceControllerStatus.Stopped, "Service did not stop as expected");
        }

        [TestMethod]
        public void ShouldStartSyncthingService()
        {
            if (!isSyncthingInstalled())
            {
                ShouldInstallSyncthingService();
            }
            var startSyncthingWrapper = new SyncthingServiceWrapper.SyncthingService();
            ServiceController service = new ServiceController(startSyncthingWrapper.ServiceName);
            if (service.Status == ServiceControllerStatus.Stopped)
            {
                service.Start();
            }
            service.WaitForStatus(ServiceControllerStatus.Running, new TimeSpan(0, 0, 10));
            Assert.IsTrue(service.Status == ServiceControllerStatus.Running, "Service did not start as expected");
        }

        [TestMethod]
        public void ShouldStartAndStopSyncthingService()
        {
            if (!isSyncthingInstalled())
            {
                ShouldInstallSyncthingService();
            }
            var startSyncthingWrapper = new SyncthingServiceWrapper.SyncthingService();
            ServiceController service = new ServiceController(startSyncthingWrapper.ServiceName);
            if (service.Status == ServiceControllerStatus.Stopped)
            {
                service.Start();
            }
            service.WaitForStatus(ServiceControllerStatus.Running, new TimeSpan(0, 0, 10));
            service.Stop();
            service.WaitForStatus(ServiceControllerStatus.Stopped, new TimeSpan(0, 0, 10));
            Assert.IsTrue(service.Status == ServiceControllerStatus.Stopped, "Service did not start and stop as expected");
        }

        private bool isSyncthingInstalled()
        {
            string registry_key = @"SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall";
            if (Environment.Is64BitOperatingSystem)
                using (RegistryKey localMachineRegistry64 = RegistryKey.OpenBaseKey(RegistryHive.LocalMachine, RegistryView.Registry64))
                {
                    using (RegistryKey key = localMachineRegistry64.OpenSubKey(registry_key))
                        foreach (string subkey_name in key.GetSubKeyNames())
                        {
                            using (RegistryKey subkey = key.OpenSubKey(subkey_name))
                            {
                                if (subkey.GetValue("DisplayName") != null)
                                    if (subkey.GetValue("DisplayName").ToString() == "Syncthing Service")
                                        return true;
                            }
                        }
                }

            using (RegistryKey localMachineRegistry32 = RegistryKey.OpenBaseKey(RegistryHive.LocalMachine, RegistryView.Registry32))
            {
                using (RegistryKey key = localMachineRegistry32.OpenSubKey(registry_key))
                    foreach (string subkey_name in key.GetSubKeyNames())
                    {
                        using (RegistryKey subkey = key.OpenSubKey(subkey_name))
                        {
                            if (subkey.GetValue("DisplayName") != null)
                                if (subkey.GetValue("DisplayName").ToString()=="Syncthing Service")
                                    return true;
                        }
                    }
            }
            return false;
        }

        [TestMethod]
        public void ShouldInstallSyncthingService()
        {
            Installer.SetInternalUI(InstallUIOptions.ProgressOnly);
            Installer.InstallProduct(msiFilename, "ACTION=INSTALL ALLUSERS=2 MSIINSTALLPERUSER=");
            var startSyncthingWrapper = new SyncthingServiceWrapper.SyncthingService();
            ServiceController service = new ServiceController(startSyncthingWrapper.ServiceName);
            service.WaitForStatus(ServiceControllerStatus.Running, new TimeSpan(0, 0, 30));
            Assert.IsTrue(service.Status == ServiceControllerStatus.Running, "Service did not start after Installing");
        }

        [TestMethod]
        public void ShouldUninstallSyncthingService()
        {
            if (!isSyncthingInstalled())
            {
                ShouldInstallSyncthingService();
            }
            Process p = new Process();
            p.StartInfo.FileName = "msiexec.exe";
            p.StartInfo.Arguments = "/x \"" + msiFilename + "\"/passive";
            p.Start();
            var startSyncthingWrapper = new SyncthingServiceWrapper.SyncthingService();
            ServiceController service = new ServiceController(startSyncthingWrapper.ServiceName);
            service.WaitForStatus(ServiceControllerStatus.Stopped, new TimeSpan(0, 0, 20));
            Assert.IsTrue(service.Status == ServiceControllerStatus.Stopped, "Service did not start after Installing");
        }

        public static bool IsUserAdministrator()
        {
            //bool value to hold our return value
            bool isAdmin;
            try
            {
                //get the currently logged in user
                WindowsIdentity user = WindowsIdentity.GetCurrent();
                WindowsPrincipal principal = new WindowsPrincipal(user);
                isAdmin = principal.IsInRole(WindowsBuiltInRole.Administrator);
            }
            catch (UnauthorizedAccessException ex)
            {
                isAdmin = false;
            }
            catch (Exception ex)
            {
                isAdmin = false;
            }
            return isAdmin;
        }
    }
}