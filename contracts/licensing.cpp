#include <eosiolib/eosio.hpp>
#include <string>
using namespace eosio;
using std::string;

class licensing: public contract {
   using contract::contract;

      public:

         //@abi action
         void addrepo(const account_name owner, string& reponame) {
            
         }

         //@abi action
         void getlicense(const account_name licto, string& reponame) {
            
         }

      private:

         //@abi table repo i64
         struct repo {
            uint64_t owner;
            string reponame;

            uint64_t primary_key() const {
               return owner;
            }

            EOSLIB_SERIALIZE(repo, (owner))
         };

         typedef multi_index<N(repo), repo> repoIndex;

         //@abi table license i64
         struct license {
            uint64_t licto;
            string reponame;

            uint64_t primary_key() const {
               return licto;
            }

            EOSLIB_SERIALIZE(license, (licto))
         };

         typedef multi_index<N(license), license> licenseIndex;
};

EOSIO_ABI(licensing, (addrepo))
