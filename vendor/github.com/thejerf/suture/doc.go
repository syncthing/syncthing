/*

Package suture provides Erlang-like supervisor trees.

This implements Erlang-esque supervisor trees, as adapted for Go. This is
intended to be an industrial-strength implementation, but it has not yet
been deployed in a hostile environment. (It's headed there, though.)

Supervisor Tree -> SuTree -> suture -> holds your code together when it's
trying to fall apart.

Why use Suture?

 * You want to write bullet-resistant services that will remain available
   despite unforeseen failure.
 * You need the code to be smart enough not to consume 100% of the CPU
   restarting things.
 * You want to easily compose multiple such services in one program.
 * You want the Erlang programmers to stop lording their supervision
   trees over you.

Suture has 100% test coverage, and is golint clean. This doesn't prove it
free of bugs, but it shows I care.

A blog post describing the design decisions is available at
http://www.jerf.org/iri/post/2930 .

Using Suture

To idiomatically use Suture, create a Supervisor which is your top level
"application" supervisor. This will often occur in your program's "main"
function.

Create "Service"s, which implement the Service interface. .Add() them
to your Supervisor. Supervisors are also services, so you can create a
tree structure here, depending on the exact combination of restarts
you want to create.

As a special case, when adding Supervisors to Supervisors, the "sub"
supervisor will have the "super" supervisor's Log function copied.
This allows you to set one log function on the "top" supervisor, and
have it propagate down to all the sub-supervisors. This also allows
libraries or modules to provide Supervisors without having to commit
their users to a particular logging method.

Finally, as what is probably the last line of your main() function, call
.Serve() on your top level supervisor. This will start all the services
you've defined.

See the Example for an example, using a simple service that serves out
incrementing integers.

*/
package suture
