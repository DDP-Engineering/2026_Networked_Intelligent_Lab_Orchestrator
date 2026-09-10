# Third-party licence inventory

The exact package-to-licence inventory is in
`../build-environment.lock.json`. A real image build copies the corresponding
package copyright texts from `/usr/share/doc/*/copyright` into the generated
licence bundle and generates the final image SBOM. This tracked directory is
the reviewable source-inventory anchor; it is not a substitute for generated
licence evidence.
