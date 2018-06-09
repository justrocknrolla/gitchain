#include <eosiolib/eosio.hpp>
#include <string>
using namespace eosio;
using std::string;

class licensing: public contract {
   using contract::contract;

      public:

         //@abi action
         void addrepo(const account_name owner, string& reponame, uint64_t licprice) {
           /**
            * We require that only the owner of an account can use this action
            * or somebody with the account authorization
            */
            require_auth(owner);

           /**
            * We access the "repo" table as creating an object of type "repoIndex"
            * As parameters we pass code & scope - _self from the parent contract
            */
            repoIndex repos(_self, _self);

           /**
            * We add the new player in the table
            * The first argument is the payer of the storage which will store the data
            */
            repos.emplace(owner, [&](auto& repo) {
               repo.owner = owner;
               repo.reponame = reponame;
               repo.licprice = licprice;
            });

         }

         //@abi action
         void createlicense(const account_name licto, string& reponame) {
           /**
            * We require that only the owner of an account can use this action
            * or somebody with the account authorization
            */
            require_auth(licto);

           /**
            * We access the "repo" table as creating an object of type "repoIndex"
            * As parameters we pass code & scope - _self from the parent contract
            */
            licenseIndex lics(_self, _self);

           /**
            * We add the new player in the table
            * The first argument is the payer of the storage which will store the data
            */
            lics.emplace(licto, [&](auto& lic) {
               lic.licto = licto;
               lic.reponame = reponame;
            });

         }

      private:

         //@abi table repo i64
         struct repo {
            uint64_t owner;
            string reponame;
            uint64_t licprice;

            uint64_t primary_key() const {
               return owner;
            }

            EOSLIB_SERIALIZE(repo, (owner)(reponame)(licprice))
         };

         typedef multi_index<N(repo), repo> repoIndex;

         //@abi table license i64
         struct license {
            uint64_t licto;
            string reponame;

            uint64_t primary_key() const {
               return licto;
            }

            EOSLIB_SERIALIZE(license, (licto)(reponame))
         };

         typedef multi_index<N(license), license> licenseIndex;
};

EOSIO_ABI(licensing, (addrepo))
