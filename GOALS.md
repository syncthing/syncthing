# The Syncthing Goals

Syncthing is a **continuous file synchronization program**. It synchronizes
files between two or more computers. We strive to fulfill the goals below.
The goals are listed in order of importance, the most important one being
the first.

> "Syncing files" here is precise. It means we specifically exclude things
> that are not files - calendar items, instant messages, and so on. If those
> are in fact stored as files on disk, they can of course be synced as
> files.

Syncthing should be:

### 1. Safe From Data Loss

Protecting the user's data is paramount. We take every reasonable precaution
to avoid corrupting the user's files.

> This is the overriding goal, without which synchronizing files becomes
> pointless. This means that we do not make unsafe trade offs for the sake
> of performance or, in some cases, even usability.

### 2. Secure Against Attackers

Again, protecting the user's data is paramount. Regardless of our other
goals we must never allow the user's data to be susceptible to eavesdropping
or modification by unauthorized parties.

> This should be understood in context. It is not necessarily reasonable to
> expect Syncthing to be resistant against well equipped state level
> attackers. We will however do our best. Note also that this is different
> from anonymity which is not, currently, a goal.

### 3. Easy to Use

Syncthing should be approachable, understandable and inclusive.

> Complex concepts and maths form the base of Syncthing's functionality.
> This should nonetheless be abstracted or hidden to a degree where
> Syncthing is usable by the general public.

### 4. Automatic

User interaction should be required only when absolutely necessary.

> Specifically this means that changes to files are picked up without
> prompting, conflicts are resolved without prompting and connections are
> maintained without prompting. We only prompt the user when it is required
> to fulfill one of the (overriding) Secure, Safe or Easy goals.

### 5. Universally Available

Syncthing should run on every common computer. We are mindful that the
latest technology is not always available to any given individual.

> Computers include desktops, laptops, servers, virtual machines, small
> general purpose computers such as Raspberry Pis and, *where possible*,
> tablets and phones. NAS appliances, toasters, cars, firearms, thermostats
> and so on may include computing capabilities but it is not our goal for
> Syncthing to run smoothly on these devices.

### 6. For Individuals

Syncthing is primarily about empowering the individual user with safe,
secure and easy to use file synchronization.

> We acknowledge that it's also useful in an enterprise setting and include
> functionality to support that. If this is in conflict with the
> requirements of the individual, those will however take priority.

### 7. Everything Else

There are many things we care about that don't make it on to the list. It is
fine to optimize for these values as well, as long as they are not in
conflict with the stated goals above.

> For example, performance is a thing we care about. We just don't care more
> about it than safety, security, etc. Maintainability of the code base and
> providing entertainment value for the maintainers are also things that
> matter. It is understood that there are aspects of Syncthing that are
> suboptimal or even in opposition with the goals above. However, we
> continuously strive to align Syncthing more and more with these goals.
