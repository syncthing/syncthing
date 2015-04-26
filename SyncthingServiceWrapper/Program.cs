using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Management;
using System.Reflection;
using System.Runtime.CompilerServices;
using System.ServiceProcess;
using System.Text;
using System.Threading;
using System.Threading.Tasks;

//show internal classes to unittests
[assembly: InternalsVisibleTo("UnitTestSyncthingServiceWrapper")]

namespace SyncthingServiceWrapper
{
    internal class SyncthingService : ServiceBase
    {
        /// <summary>
        /// Public Constructor for WindowsService.
        /// - Put all of your Initialization code here.
        /// </summary>
        public SyncthingService()
        {
            this.ServiceName = "Syncthing Service";
            this.EventLog.Log = "Application";
            this.EventLog.Source = this.ServiceName;

            // These Flags set whether or not to handle that specific
            //  type of event. Set to true if you need it, false otherwise.
            this.CanHandlePowerEvent = false;
            this.CanHandleSessionChangeEvent = false;
            //Need to implement Restapi to get PauseAndContinue working
            this.CanPauseAndContinue = false;
            this.CanShutdown = true;
            this.CanStop = true;
        }

        /// <summary>
        /// The Main Thread: This is where your Service is Run.
        /// </summary>
        private static void Main()
        {
            ServiceBase.Run(new SyncthingService());
        }

        /// <summary>
        /// Dispose of objects that need it here.
        /// </summary>
        /// <param name="disposing">Whether
        ///    or not disposing is going on.</param>
        protected override void Dispose(bool disposing)
        {
            base.Dispose(disposing);
        }

        private Process syncthingProcess;

        /// <summary>
        /// OnStart(): Put startup code here
        /// - Start threads, get inital data, etc.
        /// </summary>
        /// <param name="args">Data passed by the start command.</param>
        protected override void OnStart(string[] args)
        {
            syncthingProcess = new Process();
            syncthingProcess.StartInfo.FileName = Path.Combine(GetCurrentExecutingDirectory(), "syncthing.exe");
            syncthingProcess.StartInfo.RedirectStandardOutput = true;
            syncthingProcess.StartInfo.Arguments = SyncthingServiceWrapper.Properties.Settings.Default.syncthingArguments;
            syncthingProcess.StartInfo.RedirectStandardError = true;
            syncthingProcess.StartInfo.RedirectStandardInput = true;
            syncthingProcess.StartInfo.UseShellExecute = false;
            syncthingProcess.EnableRaisingEvents = true;
            syncthingProcess.StartInfo.CreateNoWindow = false;
            syncthingProcess.OutputDataReceived += new DataReceivedEventHandler(proc_OutputDataReceived);
            syncthingProcess.ErrorDataReceived += new DataReceivedEventHandler(proc_ErrorDataReceived);
            syncthingProcess.Exited += new EventHandler(proc_SyncthingExited);
            syncthingProcess.Start();
            base.OnStart(args);
        }

        /// <summary>
        /// Handles the SyncthingExited event of the proc control. Syncthing.exe starts itself in a new process when started. Exiting the invoking process is an event which can be waiting for before marking service as started up.
        /// </summary>
        /// <param name="sender">The source of the event.</param>
        /// <param name="e">The <see cref="EventArgs"/> instance containing the event data.</param>
        private void proc_SyncthingExited(object sender, EventArgs e)
        {
            var timeout = DateTime.Now;
            while (getSyncthingExeProcesses().Count() == 0 || DateTime.Compare(timeout.AddMilliseconds(15000), DateTime.Now) == -1)
            {
                System.Threading.Thread.Sleep(100);
            }

            if (getSyncthingExeProcesses().Count() > 0)
            {
                this.EventLog.WriteEntry("Syncthing.exe starter fisnished", EventLogEntryType.Warning);
                //todo: add waiting for startup https://github.com/syncthing/syncthing/wiki/REST-Interface
            }
            else
            {
                this.EventLog.WriteEntry("Syncthing.exe crashed", EventLogEntryType.Error);
                this.Stop();
            }
        }

        private Process[] getSyncthingExeProcesses()
        {
            string userName = System.Security.Principal.WindowsIdentity.GetCurrent().Name;
            return Process.GetProcessesByName("syncthing").Where(o => GetProcessOwner(o.Id) == userName).ToArray();
        }

        /// <summary>
        /// Gets the process owner.
        /// </summary>
        /// <param name="processId">The process identifier.</param>
        /// <returns></returns>
        private static string GetProcessOwner(int processId)
        {
            string query = "Select * From Win32_Process Where ProcessID = " + processId;
            ManagementObjectSearcher searcher = new ManagementObjectSearcher(query);
            ManagementObjectCollection processList = searcher.Get();

            foreach (ManagementObject obj in processList)
            {
                string[] argList = new string[] { string.Empty, string.Empty };
                int returnVal = Convert.ToInt32(obj.InvokeMethod("GetOwner", argList));
                if (returnVal == 0)
                {
                    // return DOMAIN\user
                    return argList[1] + "\\" + argList[0];
                }
            }

            return "NO OWNER";
        }

        /// <summary>
        /// Handles the ErrorDataReceived event of the proc control.
        /// </summary>
        /// <param name="sender">The source of the event.</param>
        /// <param name="e">The <see cref="DataReceivedEventArgs"/> instance containing the event data.</param>
        private void proc_ErrorDataReceived(object sender, DataReceivedEventArgs e)
        {
            this.EventLog.WriteEntry(e.Data);
        }

        /// <summary>
        /// Handles the OutputDataReceived event of the proc control.
        /// </summary>
        /// <param name="sender">The source of the event.</param>
        /// <param name="e">The <see cref="DataReceivedEventArgs"/> instance containing the event data.</param>
        private void proc_OutputDataReceived(object sender, DataReceivedEventArgs e)
        {
            this.EventLog.WriteEntry(e.Data);
        }

        /// <summary>
        /// Gets the current executing directory.
        /// </summary>
        /// <returns></returns>
        private static string GetCurrentExecutingDirectory()
        {
            return Path.GetDirectoryName(new System.Uri(System.Reflection.Assembly.GetExecutingAssembly().CodeBase).LocalPath);
        }

        /// <summary>
        /// OnStop(): Put your stop code here
        /// - Stop threads, set final data, etc.
        /// </summary>
        protected override void OnStop()
        {
            shutdownSyncthingExe();
            base.OnStop();
        }

        /// <summary>
        /// Shutdowns the syncthing executable.
        /// </summary>
        private void shutdownSyncthingExe()
        {
            this.EventLog.WriteEntry("Shuting down Syncthing ", EventLogEntryType.Information);
            Thread.Sleep(300);
            foreach (var item in getSyncthingExeProcesses())
            {
                if (!item.HasExited)
                {
                    this.EventLog.WriteEntry("Killing Syncthing with pid " + item.Id, EventLogEntryType.Information);
                    item.Kill();
                    Thread.Sleep(300);
                }
            }
            Thread.Sleep(300);
            foreach (var item in getSyncthingExeProcesses())
            {
                if (!item.HasExited)
                {
                    this.EventLog.WriteEntry("Killing -still running syncthing with pid " + item.Id, EventLogEntryType.Warning);
                    item.Kill();
                    Thread.Sleep(300);
                }
            }
            //syncthingProcess.Dispose();
            this.EventLog.WriteEntry("Shutting down Syncthing finished", EventLogEntryType.Information);
        }

        /// <summary>
        /// OnPause: Put your pause code here
        /// - Pause working threads, etc.
        /// </summary>
        protected override void OnPause()
        {
            base.OnPause();
        }

        /// <summary>
        /// OnContinue(): Put your continue code here
        /// - Un-pause working threads, etc.
        /// </summary>
        protected override void OnContinue()
        {
            base.OnContinue();
        }

        /// <summary>
        /// OnShutdown(): Called when the System is shutting down
        /// - Put code here when you need special handling
        ///   of code that deals with a system shutdown, such
        ///   as saving special data before shutdown.
        /// </summary>
        protected override void OnShutdown()
        {
            shutdownSyncthingExe();
            base.OnShutdown();
        }

        /// <summary>
        /// OnCustomCommand(): If you need to send a command to your
        ///   service without the need for Remoting or Sockets, use
        ///   this method to do custom methods.
        /// </summary>
        /// <param name="command">Arbitrary Integer between 128 & 256</param>
        protected override void OnCustomCommand(int command)
        {
            //  A custom command can be sent to a service by using this method:
            //#  int command = 128; //Some Arbitrary number between 128 & 256
            //#  ServiceController sc = new ServiceController("NameOfService");
            //#  sc.ExecuteCommand(command);

            base.OnCustomCommand(command);
        }

        /// <summary>
        /// OnPowerEvent(): Useful for detecting power status changes,
        ///   such as going into Suspend mode or Low Battery for laptops.
        /// </summary>
        /// <param name="powerStatus">The Power Broadcast Status
        /// (BatteryLow, Suspend, etc.)</param>
        protected override bool OnPowerEvent(PowerBroadcastStatus powerStatus)
        {
            return base.OnPowerEvent(powerStatus);
        }

        /// <summary>
        /// OnSessionChange(): To handle a change event
        ///   from a Terminal Server session.
        ///   Useful if you need to determine
        ///   when a user logs in remotely or logs off,
        ///   or when someone logs into the console.
        /// </summary>
        /// <param name="changeDescription">The Session Change
        /// Event that occured.</param>
        protected override void OnSessionChange(
                  SessionChangeDescription changeDescription)
        {
            base.OnSessionChange(changeDescription);
        }
    }
}